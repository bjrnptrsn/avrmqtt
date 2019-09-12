package avr

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/JohannWeging/logerr"
	"github.com/juju/errors"
	log "github.com/sirupsen/logrus"
	"github.com/ziutek/telnet"
)

type Event struct {
	Data string
}

type AVR struct {
	m      sync.Mutex
	opts   *Options
	telnet *telnet.Conn
	Events chan *Event
	state  map[string]string
	logger *log.Entry
}

type Options struct {
	Host         string
	TelnetPort   string
	telnetHost   string
}

func New(opts *Options) *AVR {
	opts.telnetHost = fmt.Sprintf("%s:%s", opts.Host, opts.TelnetPort)
	avr := &AVR{
		opts:   opts,
		Events: make(chan *Event),
		state:  make(map[string]string),
	}
	avr.logger = log.WithFields(avr.logFields())

	go avr.listenTelnet()
	return avr
}

func (a *AVR) logFields() map[string]interface{} {
	return map[string]interface{}{
		"module": "avr",
		"telnet": a.opts.telnetHost,
	}
}

func (a *AVR) listenTelnet() {
	var err error
	for {
		a.telnet, err = telnet.DialTimeout("tcp", a.opts.telnetHost, 5*time.Second)
		if err != nil {
			// this is set to info because if the receiver is powered down
			// is can spam logs
			a.logger.WithError(err).Info("failed to connect to telnet")
			time.Sleep(5 * time.Second)
			continue
		}
		if err = a.telnet.Conn.(*net.TCPConn).SetKeepAlive(true); err != nil {
			a.logger.WithError(err).Error("failed to enable tcp keep alive")
		}
		if err = a.telnet.Conn.(*net.TCPConn).SetKeepAlivePeriod(5 * time.Second); err != nil {
			a.logger.WithError(err).Error("failed to set tcp keep alive period")
		}
		a.logger.Debug("telnet connected")
		go a.setState()
		for {
			data, err := a.telnet.ReadString('\r')
			if err != nil {
				a.logger.WithError(err).Errorf("failed to read form telnet")
				break
			}
			data = strings.Trim(data, " \n\r")
			a.logger.WithField("data", data).Debug("recived data")
			a.Events <- &Event{Data: data}
		}
	}
}

func (a *AVR) setState() {
	time.Sleep(3 * time.Second)
	for key, value := range a.state {
		if err := a.Command(key, value); err != nil {
			fields := logerr.GetFields(err)
			log.WithFields(fields).WithError(err).Error("failed to send telnet command")
		}
	}
}

func (a *AVR) Command(endpoint, payload string) error {
	a.m.Lock()
	defer a.m.Unlock()
	a.state[endpoint] = payload
	cmd := ""
	if strings.HasPrefix(endpoint, "PS") || strings.HasPrefix(endpoint, "CV") {
		if endpoint == "PSMODE" {
			cmd = endpoint + ":" + payload
		} else {
			cmd = endpoint + " " + payload
		}
	} else {
		cmd = endpoint + payload
	}
	a.logger.WithField("cmd", cmd).Debug("send telnet command")
	err := a.sendTelnet(cmd)
	if err != nil {
		lf := a.logFields()
		lf["cmd"] = cmd
		err = logerr.WithFields(err, lf)
		return errors.Annotate(err, "failed to send cmd")
	}
	time.Sleep(100 * time.Millisecond)
	return nil
}

func (a *AVR) sendTelnet(cmd string) error {
	var err error
	if a.telnet == nil {
		a.telnet, err = telnet.DialTimeout("tcp", a.opts.telnetHost, 5*time.Second)
		if err != nil {
			return errors.Annotate(err, "failed to connect to telnet")
		}
	}
	_, err = a.telnet.Write([]byte(cmd))
	return errors.Annotate(err, "failed to do telnet request")
}
