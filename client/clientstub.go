package client

import (
	"fmt"
	"if-frame/internal/message"
	"if-frame/internal/network"
	"net"
	"time"
)

type ClientMsgCallback func(m *message.Message, c *ClientStub)

type ClientStub struct {
	msgCb      ClientMsgCallback
	msgChannel *network.Channel
	closed     bool
	serverAddr string
	syncChan   chan int32
}

// Create a client with callback function
func CreateClientStub(MsgCb ClientMsgCallback) *ClientStub {
	return &ClientStub{
		msgCb:      MsgCb,
		msgChannel: nil,
		closed:     true,
		serverAddr: "",
		syncChan:   make(chan int32),
	}
}

func (c *ClientStub) Start(isAsync bool) {
	for {
		conn, err := net.Dial("tcp", c.serverAddr)
		if err != nil {
			fmt.Errorf("connect %s failed, err: %s", c.serverAddr, err.Error())
			panic(err)
		} else {
			c.msgChannel = network.CreateChannel(conn, c.OnChannelMsg)
			if !isAsync && c.closed {
				// avoid reconnect would send again
				c.syncChan <- 1
			}
			c.closed = false
			c.msgChannel.Start()
			// reconnect
			if c.closed {
				return
			}
		}
		time.Sleep(5 * time.Second)
	}
}

// connet to address async
// need to make sure that is connected
func (c *ClientStub) ConnectAsyncTo(address string) {
	if address != c.serverAddr {
		c.serverAddr = address
		c.Close()
	}
	go c.Start(true)
}

// connet to address sync
func (c *ClientStub) ConnectSyncTo(address string) {
	if address != c.serverAddr {
		c.serverAddr = address
		c.Close()
	}
	go c.Start(false)
	<-c.syncChan // wait
}

// close connect
func (c *ClientStub) Close() {
	if !c.closed {
		c.closed = true
		c.msgChannel.Close()
	}
}

// send message to server,
// need to package message
func (c *ClientStub) SendMsg(m *message.Message) {
	c.msgChannel.SendMsg(m)
}

// send message to server with type,
// don't need to package message
func (c *ClientStub) Send(msgType uint32, msgLength uint32, seq uint32, msg []byte) {
	m := message.CreateMessage(msgType, msgLength, seq, msg)
	c.SendMsg(m)
}

func (c *ClientStub) OnChannelMsg(msg *message.Message) {
	c.msgCb(msg, c)
}

// get local address
func (c *ClientStub) LocalAddr() net.Addr {
	return c.msgChannel.Conn.LocalAddr()
}

// get remote address
func (c *ClientStub) RemoteAddr() net.Addr {
	return c.msgChannel.Conn.RemoteAddr()
}
