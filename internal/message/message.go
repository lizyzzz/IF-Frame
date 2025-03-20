package message

import "encoding/binary"

/*
	0-1: Magic number 0x55 0xaa
	2-5: Message Type
	6-9: Message length
	10-13: Message sequence
*/

type Message struct {
	Header  [16]byte
	Payload []byte
}

func CreateMessage(msgType uint32, msgLength uint32, seq uint32, msg []byte) *Message {
	result := &Message{}
	// 组织头部
	result.Header[0] = 0x55
	result.Header[1] = 0xaa
	binary.LittleEndian.PutUint32(result.Header[2:6], msgType)
	binary.LittleEndian.PutUint32(result.Header[6:10], msgLength)
	binary.LittleEndian.PutUint32(result.Header[10:14], seq)
	// 消息内容
	result.Payload = msg
	return result
}

func (m *Message) Type() uint32 {
	msgtype := binary.LittleEndian.Uint32(m.Header[2:6])
	return msgtype
}

func (m *Message) Length() uint32 {
	msglen := binary.LittleEndian.Uint32(m.Header[6:10])
	return msglen
}

func (m *Message) Sequence() uint32 {
	seq := binary.LittleEndian.Uint32(m.Header[10:14])
	return seq
}

func (m *Message) GetPayload() []byte {
	return m.Payload
}
