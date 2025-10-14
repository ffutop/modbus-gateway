package transport

import (
	"github.com/ffutop/modbus-gateway/modbus"
	"github.com/ffutop/modbus-gateway/transport/rtu"
)

type Client struct {
	modbus.Transporter
	modbus.Connector
}

func NewClient(rtuClientHandler *rtu.RTUClientHandler) *Client {
	return &Client{
		Transporter: rtuClientHandler,
		Connector:   rtuClientHandler,
	}
}
