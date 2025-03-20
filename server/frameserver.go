package server

import (
	"fmt"
	"if-frame/internal/message"
	pb "if-frame/internal/proto"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
)

type FrameServer struct {
	Svr            *Server
	Addr           string
	CurrentFrameId int32
	frames         []*pb.DataFrame // 历史帧
	isReady        bool
	CtrlCmds       map[int32][]*pb.CtrlCmd
	Lock           sync.Mutex
	clientmap      map[int32]*pb.Position
	Num            int
	KeyFrameTime   int32
}

func CreateFrameServer(addr string, num int) *FrameServer {
	return &FrameServer{
		Addr:           addr,
		CurrentFrameId: 0,
		frames:         make([]*pb.DataFrame, 0),
		isReady:        false,
		CtrlCmds:       make(map[int32][]*pb.CtrlCmd),
		clientmap:      make(map[int32]*pb.Position),
		Num:            num,
		KeyFrameTime:   100,
	}
}

// 匹配完成, 下发首帧
func (fs *FrameServer) SendFrame0ToClient(stub *ClientStub) {
	// 组织首帧
	frame0 := &pb.ServerFirstFrame{
		FrameId: 0, // 首帧为 0
	}
	for cId, pos := range fs.clientmap {
		frame0.Positions = append(frame0.Positions, &pb.Position{
			ClientId: cId,
			X:        pos.X,
			Y:        pos.Y,
		})
	}
	// 序列化
	dataFrame0, err := proto.Marshal(frame0)
	if err != nil {
		fmt.Printf("序列化失败: %v\n", err)
		return
	}
	dataFrame0Msg := message.CreateMessage(message.CONNECTRSPFRAME, uint32(len(dataFrame0)), 0, dataFrame0)
	// 发送首帧
	stub.SendMsg(dataFrame0Msg)
	fmt.Printf("send frame0 to client<%s> frame<%v>\n", stub.RemoteAddr().String(), frame0)
}

// 断线重连用户下发所有帧
func (fs *FrameServer) SendAllFrameToClient(stub *ClientStub) {
	fs.SendFrame0ToClient(stub)

	// 下发所有历史帧
	for _, f := range fs.frames {
		dataFrame, err := proto.Marshal(f)
		if err != nil {
			fmt.Printf("序列化失败: %v\n", err)
			return
		}
		dataFrameMsg := message.CreateMessage(message.KEYFRAME, uint32(len(dataFrame)), 0, dataFrame)

		stub.SendMsg(dataFrameMsg)
	}
}

// 发送某个指定帧
func (fs *FrameServer) SendFrameToClientById(frameId int32, stub *ClientStub) {
	// 下发指定帧
	for _, f := range fs.frames {
		if f.FrameId != frameId {
			continue
		}
		dataFrame, err := proto.Marshal(f)
		if err != nil {
			fmt.Printf("序列化失败: %v\n", err)
			return
		}
		dataFrameMsg := message.CreateMessage(message.KEYFRAME, uint32(len(dataFrame)), 0, dataFrame)

		stub.SendMsg(dataFrameMsg)
		break
	}
}

// 发送某个指定帧
func (fs *FrameServer) SendFrameToClient(frame *pb.DataFrame, stub *ClientStub) {
	// 下发指定帧
	dataFrame, err := proto.Marshal(frame)
	if err != nil {
		fmt.Printf("序列化失败: %v\n", err)
		return
	}
	dataFrameMsg := message.CreateMessage(message.KEYFRAME, uint32(len(dataFrame)), 0, dataFrame)

	stub.SendMsg(dataFrameMsg)
}

// 接受到消息的处理函数
func (fs *FrameServer) OnStubMsg(m *message.Message, stub *ClientStub) {
	fs.Lock.Lock()
	defer fs.Lock.Unlock()

	t := m.Type()
	switch t {
	case message.CONNECTFRAME:
		// 上行首帧
		connFrame := &pb.ClientFirstFrame{}
		err := proto.Unmarshal(m.Payload, connFrame)
		if err != nil {
			fmt.Printf("反序列化失败: %v\n", err)
			return
		}

		id := connFrame.Position.ClientId
		fmt.Printf("Recv connect frame from client<%d>, position (%d, %d)\n", id, connFrame.Position.X, connFrame.Position.Y)
		if len(fs.clientmap) < fs.Num {
			// 匹配中

			fs.clientmap[id] = connFrame.Position
			fs.CtrlCmds[id] = nil
			if len(fs.clientmap) == fs.Num {
				// 匹配完成, 下发首帧
				fs.isReady = true
				fmt.Printf("game ready.\n")
				for stub := range fs.Svr.clientStubs {
					fs.SendFrame0ToClient(stub)
				}
				// 帧号增加
				fs.CurrentFrameId++
			}
		} else {
			_, ok := fs.clientmap[id]
			if ok && fs.isReady {
				// 已存在的client 再次发首帧, 为断线重连
				// 下发所有帧
				fs.SendAllFrameToClient(stub)
			} else {
				// 拒绝其他客户端的连接
				stub.Close()
			}
		}

	case message.UPLOADFRAME:
		// 上行操作指令帧
		uploadFrame := &pb.CtrlCmd{}
		err := proto.Unmarshal(m.Payload, uploadFrame)
		if err != nil {
			fmt.Printf("反序列化失败: %v\n", err)
			return
		}
		id := uploadFrame.ClientId

		if len(uploadFrame.Cmd) > 0 {
			fmt.Printf("Recv upload frame from client<%d>, ctrlcmd (%v)\n", id, uploadFrame)
		}

		_, ok := fs.CtrlCmds[id]
		if !ok {
			fmt.Printf("Not find client<%d>\n", id)
			return
		}

		if uploadFrame.FirstId < fs.CurrentFrameId {
			// 下发缺失帧
			fmt.Printf("Lack frame <%d> - <%d>\n", uploadFrame.FirstId, fs.CurrentFrameId)
			for i := uploadFrame.FirstId; i < fs.CurrentFrameId; i++ {
				fs.SendFrameToClientById(i, stub)
			}
		}

		// 直接添加, 如有不连续让客户端回滚
		if len(uploadFrame.Cmd) > 0 {
			fs.CtrlCmds[id] = append(fs.CtrlCmds[id], uploadFrame)
		}

	case message.QUITFRAME:
		// 退出帧
		quitFrame := &pb.QuitFrame{}
		err := proto.Unmarshal(m.Payload, quitFrame)
		if err != nil {
			fmt.Errorf("反序列化失败: %v\n", err)
			return
		}

		fmt.Printf("Recv quit frame from client<%d>, ctrlcmd (%v)\n", quitFrame.ClientId, quitFrame)

		// id := quitFrame.ClientId
		stub.Close()
	}
}

func (fs *FrameServer) Run() {
	// 定时下发帧
	go func() {
		for {
			if !fs.isReady {
				time.Sleep(time.Millisecond * 2)
				continue
			}

			// 定时下发关键帧
			select {
			case <-time.After(time.Duration(fs.KeyFrameTime) * time.Millisecond):
				fs.Lock.Lock()
				if len(fs.Svr.clientStubs) == 0 {
					fs.Lock.Unlock()
					break
				}

				// 组织关键帧
				flag := false
				frame := &pb.DataFrame{
					FrameId:    fs.CurrentFrameId,
					ClientCmds: make([]*pb.ClientCmd, 0),
				}
				for id, f := range fs.CtrlCmds {
					clientCmd := &pb.ClientCmd{
						ClientId: id,
						Cmds:     nil,
					}

					clientCmd.Cmds = append(clientCmd.Cmds, f...)

					if clientCmd.Cmds != nil || len(clientCmd.Cmds) > 0 {
						flag = true
					}

					frame.ClientCmds = append(frame.ClientCmds, clientCmd)

					// 清空
					fs.CtrlCmds[id] = nil
				}

				fs.frames = append(fs.frames, frame)

				// 下发帧
				for stub := range fs.Svr.clientStubs {
					fs.SendFrameToClient(frame, stub)
				}
				if flag {
					fmt.Printf("send key frame<%d>, frame: %v\n", fs.CurrentFrameId, frame)
				}
				fs.CurrentFrameId++
				fs.Lock.Unlock()
			}

		}
	}()
	fs.Svr = CreateServer(fs.Addr, fs.OnStubMsg)
	fs.Svr.Start()
}
