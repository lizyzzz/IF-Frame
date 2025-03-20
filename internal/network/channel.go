package network

import (
	"if-frame/internal/message"
	"net"
)

type MsgCallback func(msg *message.Message)

type Channel struct {
	closed       bool
	Conn         net.Conn
	msgCb        MsgCallback
	errorChannel chan error
	readChannel  chan *message.Message
	writeChannel chan *message.Message
	quitChannel  chan int32
}

func CreateChannel(Conn net.Conn, msgCb MsgCallback) *Channel {
	c := &Channel{
		closed:       false,
		Conn:         Conn,
		msgCb:        msgCb,
		errorChannel: make(chan error, 2),
		readChannel:  make(chan *message.Message, 128),
		writeChannel: make(chan *message.Message, 128),
		quitChannel:  make(chan int32, 1),
	}
	return c
}

// 主循环
func (c *Channel) Start() {
	go c.sendingLoop()
	go c.readingLoop()

outLoop:
	for {
		select {
		case msg := <-c.readChannel:
			c.msgCb(msg)
		case <-c.errorChannel:
			break outLoop
		}
	}

	c.Close()
}

func (c *Channel) sendingLoop() {
	for {
		select {
		case msg := <-c.writeChannel:
			// send header
			headerSend := 0
			for headerSend < 16 {
				n, err := c.Conn.Write(msg.Header[headerSend:])
				if err != nil {
					c.errorChannel <- err
					return
				}
				headerSend += n
			}

			// send payload
			byteLeft := msg.Length()
			byteTotal := byteLeft
			for byteLeft > 0 {
				n, err := c.Conn.Write(msg.Payload[byteTotal-byteLeft:])
				if err != nil {
					c.errorChannel <- err
					return
				}
				byteLeft -= uint32(n)
			}
		case <-c.quitChannel:
			return
		}
	}
}

func (c *Channel) readingLoop() {
	for {
		// read header
		msg := &message.Message{}
		headerRead := 0
		for headerRead < 16 {
			n, err := c.Conn.Read(msg.Header[headerRead:])
			if err != nil {
				c.errorChannel <- err
				return
			}
			headerRead += n
		}

		// read payload
		msglen := msg.Length()
		msg.Payload = make([]byte, msglen)
		var totalRead uint32 = 0
		for totalRead < msglen {
			n, err := c.Conn.Read(msg.Payload[totalRead:])
			if err != nil {
				c.errorChannel <- err
				return
			}
			totalRead += uint32(n)
		}

		c.readChannel <- msg
	}
}

func (c *Channel) SendMsg(msg *message.Message) {
	if !c.closed {
		c.writeChannel <- msg
	}
}

func (c *Channel) Close() {
	if !c.closed {
		c.closed = true
		c.Conn.Close()
		c.quitChannel <- 1
	}
}
