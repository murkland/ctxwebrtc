package ctxwebrtc

import (
	"context"
	"net"

	"github.com/pion/webrtc/v3"
)

type DataChannel struct {
	dc            *webrtc.DataChannel
	err           error
	ready         chan struct{}
	closed        chan struct{}
	sendMore      chan struct{}
	recvBuf       chan []byte
	lowWaterMark  uint
	highWaterMark uint
}

func (c *DataChannel) Send(ctx context.Context, buf []byte) error {
	select {
	case <-c.closed:
		if c.err != nil {
			return c.err
		}
		return net.ErrClosed
	case <-c.ready:
	case <-ctx.Done():
		return ctx.Err()
	}
	if c.dc.BufferedAmount()+uint64(len(buf)) > uint64(c.highWaterMark) {
		select {
		case <-c.closed:
			return net.ErrClosed
		case <-ctx.Done():
			return ctx.Err()
		case <-c.sendMore:
		}
	}
	return c.dc.Send(buf)
}

func (c *DataChannel) Recv(ctx context.Context) ([]byte, error) {
	select {
	case <-c.closed:
		if c.err != nil {
			return nil, c.err
		}
		return nil, net.ErrClosed
	case buf := <-c.recvBuf:
		return buf, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *DataChannel) Close() error {
	return c.dc.Close()
}

const defaultLowWaterMark = 512 * 1024   // 512KB
const defaultHighWaterMark = 1024 * 1024 // 1MB

func WrapDataChannel(dc *webrtc.DataChannel, options ...func(*DataChannel)) *DataChannel {
	ch := &DataChannel{dc, nil, make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan []byte), defaultLowWaterMark, defaultHighWaterMark}
	for _, opt := range options {
		opt(ch)
	}
	ch.dc.SetBufferedAmountLowThreshold(uint64(ch.lowWaterMark))
	ch.dc.OnOpen(func() {
		close(ch.ready)
	})
	ch.dc.OnError(func(err error) {
		ch.err = err
		close(ch.closed)
	})
	ch.dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		ch.recvBuf <- msg.Data
	})
	ch.dc.OnBufferedAmountLow(func() {
		ch.sendMore <- struct{}{}
	})
	ch.dc.OnClose(func() {
		close(ch.closed)
	})
	return ch
}

func WithHighWaterMark(highWaterMark uint) func(*DataChannel) {
	return func(c *DataChannel) {
		c.highWaterMark = highWaterMark
	}
}

func WithLowWaterMark(lowWaterMark uint) func(*DataChannel) {
	return func(c *DataChannel) {
		c.lowWaterMark = lowWaterMark
	}
}
