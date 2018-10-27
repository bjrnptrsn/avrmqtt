package avr

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/JohannWeging/logerr"
	"github.com/juju/errors"
	log "github.com/sirupsen/logrus"
	"github.com/ziutek/telnet"
)

const (
	urlAppDirekt = "/goform/formiPhoneAppDirect.xml"
)

var longCommand = []string{"SLP", "NSA", "NSE"}

type Event struct {
	Type  string
	Value string
}

type AVR struct {
	opts   *Options
	http   *http.Client
	telnet *telnet.Conn
	Events chan Event
}

type Options struct {
	Host         string
	HttpPort     string
	TelnetPort   string
	httpEndpoint string
}

func New(opts *Options) *AVR {
	opts.httpEndpoint = fmt.Sprintf("http://%s:%s%s", opts.Host, opts.HttpPort, urlAppDirekt)
	avr := &AVR{
		opts:   opts,
		http:   http.DefaultClient,
		Events: make(chan Event),
	}

	go avr.listenTelnet()
	return avr
}

func (a *AVR) listenTelnet() {
	telnetHost := fmt.Sprintf("%s:%s", a.opts.Host, a.opts.TelnetPort)
	fields := log.Fields{
		"telnet_host": telnetHost,
		"module":      "telnet",
	}
	logger := log.WithFields(fields)
	var err error
	for {
		a.telnet, err = telnet.DialTimeout("tcp", telnetHost, 5*time.Second)
		if err != nil {
			logger.WithError(err).Error("failed to connect to telnet")
			time.Sleep(10 * time.Second)
			continue
		}
		logger.Debug("telnet connected")
		for {
			data, err := a.telnet.ReadString('\r')
			if err != nil {
				logger.Errorf("failed to read form telnet")
				break
			}
			data = strings.Trim(data, " \n\r")
			logger.WithField("data", data).Debug("recived data")
			a.Events <- parseData(data)
		}
	}
}

func parseData(data string) Event {
	normalCmd := true
	typ := ""

	if strings.HasPrefix(data, "Z2") {
		typ = "Z2"
		data = data[2:]
	}
	for _, lcmd := range longCommand {
		if strings.HasPrefix(data, lcmd) {
			typ += lcmd
			data = data[3:]
			normalCmd = false
		}
	}

	if strings.HasPrefix(data, "CV") {
		typ += data[:2]
		data = data[2:]
		t, d := parseCVCmd(data)
		typ += t
		data = d
		normalCmd = false
	}

	if strings.HasPrefix(data, "PS") {
		typ += data[:2]
		data = data[2:]
		t, d := parsePSCmd(data)
		typ += t
		data = d
		normalCmd = false
	}

	if normalCmd {
		typ += data[:2]
		data = data[2:]
	}

	return Event{
		Type:  typ,
		Value: data,
	}
}

func parseCVCmd(data string) (string, string) {
	parts := strings.Fields(data)
	return parts[0], strings.Join(parts[1:], " ")
}

func parsePSCmd(data string) (string, string) {
	if strings.HasPrefix(data, "MODE") || strings.HasPrefix(data, "MULTEQ") {
		parts := strings.Split(data, ":")
		typ := parts[0]
		data = parts[1]
		return typ, data
	}

	parts := strings.Fields(data)
	typ := parts[0]
	data = strings.Join(parts[1:], " ")
	return typ, data
}

func (a *AVR) Command(cmd string) error {
	err := get(a.http, a.opts.httpEndpoint, cmd)
	err = logerr.WithFields(err,
		logerr.Fields{
			"cmd":    cmd,
			"module": "telnet",
		},
	)

	return errors.Annotate(err, "failed to send cmd")
}

func get(client *http.Client, endpoint string, cmd string) error {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		err = logerr.WithField(err, "url", endpoint)
		err = errors.Annotate(err, "failed to create request")
		return err
	}

	// add the command as empty parameter
	req.URL.RawQuery = url.QueryEscape(cmd)
	log.WithFields(log.Fields{
		"module": "telnet",
		"url":    req.URL.String(),
	}).Debug("send http request")
	_, err = client.Do(req)
	err = logerr.WithField(err, "url", req.URL.String())
	return errors.Annotate(err, "failed to do request")
}