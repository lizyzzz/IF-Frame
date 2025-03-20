package client

import (
	"if-frame/internal/message"
	pb "if-frame/internal/proto"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gammazero/deque"
	"github.com/nsf/termbox-go"
	"google.golang.org/protobuf/proto"
)

type Client struct {
	Client_id     int32
	Position      *pb.Position
	lastPosition  *pb.Position // 用来回滚
	Others        []*pb.Position
	Cmd           deque.Deque[byte]
	Lock          sync.Mutex
	EventChan     chan byte
	RenderTime    int32
	UpLoadTime    int32
	Stub          *ClientStub
	RemoteAddr    string
	isReady       bool
	isConnected   bool
	firstFrameId  int32
	secondFrameId int32
}

func CreateNewClient(client_id, position_x, position_y int32, addr string) *Client {
	result := &Client{
		Client_id: client_id,
		Position: &pb.Position{
			ClientId: client_id,
			X:        position_x,
			Y:        position_y,
		},
		lastPosition: &pb.Position{
			ClientId: client_id,
			X:        position_x,
			Y:        position_y,
		},
		Others:        make([]*pb.Position, 0),
		EventChan:     make(chan byte),
		RenderTime:    20,
		UpLoadTime:    30,
		RemoteAddr:    addr,
		isReady:       false,
		isConnected:   false,
		firstFrameId:  0,
		secondFrameId: 0,
	}
	return result
}

func (c *Client) GetCmd() {
	// 监听键盘事件
	for {
		event := termbox.PollEvent()
		switch event.Type {
		case termbox.EventKey:
			// 按键事件
			switch event.Key {
			case termbox.KeyEsc:
				c.EventChan <- byte(termbox.KeyEsc)
				return
			default:
				// 打印按键的字符
				if event.Ch == 'W' || event.Ch == 'w' ||
					event.Ch == 'A' || event.Ch == 'a' ||
					event.Ch == 'S' || event.Ch == 's' ||
					event.Ch == 'D' || event.Ch == 'd' {
					c.Lock.Lock()
					c.Cmd.PushBack(byte(event.Ch))
					c.Lock.Unlock()
					c.EventChan <- byte(event.Ch)
				}
			}
		case termbox.EventError:
			log.Println("Error:", event.Err)
			return
		}
	}
}

func (c *Client) PhysicalModel(cmd string, pos *pb.Position) *pb.Position {
	r := &pb.Position{
		ClientId: pos.ClientId,
		X:        pos.X,
		Y:        pos.Y,
	}
	for _, event := range cmd {
		switch event {
		case 'W':
		case 'w':
			if r.GetY() > 1 {
				r.Y--
			}
		case 'S':
		case 's':
			if r.GetY() < 18 {
				r.Y++
			}
		case 'A':
		case 'a':
			if r.GetX() > 1 {
				r.X--
			}
		case 'D':
		case 'd':
			if r.GetX() < 38 {
				r.X++
			}
		default:
			return r
		}
	}
	return r
}

// 绘图
func (c *Client) Render() {

	// 清屏
	termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)

	// 绘制边界
	for x := 0; x < 40; x++ {
		termbox.SetCell(x, 0, '*', termbox.ColorDefault, termbox.ColorDefault)
		termbox.SetCell(x, 19, '*', termbox.ColorDefault, termbox.ColorDefault)
	}
	for y := 0; y < 20; y++ {
		termbox.SetCell(0, y, '*', termbox.ColorDefault, termbox.ColorDefault)
		termbox.SetCell(39, y, '*', termbox.ColorDefault, termbox.ColorDefault)
	}

	c.Lock.Lock()
	defer c.Lock.Unlock()
	// 绘制目标
	for _, target := range c.Others {
		termbox.SetCell(int(target.GetX()), int(target.GetY()), 'O', termbox.ColorGreen, termbox.ColorDefault)
	}

	// 绘制玩家
	termbox.SetCell(int(c.Position.GetX()), int(c.Position.GetY()), '@', termbox.ColorBlue, termbox.ColorDefault)

	// 刷新显示
	termbox.Flush()
}

// 接受消息回调函数
func (c *Client) OnMsg(msg *message.Message, stub *ClientStub) {
	c.Lock.Lock()
	defer c.Lock.Unlock()

	// 反序列化消息.
	t := msg.Type()
	switch t {
	case message.CONNECTRSPFRAME:
		// 连接帧
		connRspFrame := &pb.ServerFirstFrame{}
		err := proto.Unmarshal(msg.Payload, connRspFrame)
		if err != nil {
			log.Fatalf("反序列化失败: %v\n", err)
			return
		}
		log.Printf("recv conn rsp frame<%v>\n", connRspFrame)

		// 初始化其他玩家的位置
		c.isReady = true
		c.isConnected = true
		for _, p := range connRspFrame.Positions {
			if p.ClientId == c.Client_id {
				continue
			}
			c.Others = append(c.Others, p)
		}

		c.firstFrameId = connRspFrame.FrameId + 1
		c.secondFrameId = 0
		c.lastPosition.X = c.Position.X
		c.lastPosition.Y = c.Position.Y

	case message.KEYFRAME:
		// 关键帧

		keyFrame := &pb.DataFrame{}
		err := proto.Unmarshal(msg.Payload, keyFrame)
		if err != nil {
			log.Fatalf("反序列化失败: %v\n", err)
			return
		}

		if len(keyFrame.ClientCmds) > 1 {
			if (len(keyFrame.ClientCmds[0].Cmds) > 0 && len(keyFrame.ClientCmds[0].Cmds[0].Cmd) > 0) ||
				(len(keyFrame.ClientCmds[1].Cmds) > 0 && len(keyFrame.ClientCmds[1].Cmds[0].Cmd) > 0) {
				log.Printf("Recv key frame<%v>\n", keyFrame)
			}
		}

		// 丢弃帧
		if c.firstFrameId != keyFrame.FrameId {
			log.Printf("Current FrameId<%d>, Frame<%d><%v> is out\n", c.firstFrameId, keyFrame.FrameId, keyFrame)
			return
		}

		// 运动模型
		c.Update(keyFrame)
		c.firstFrameId++
		c.secondFrameId = 0
	}
}

func (c *Client) Run() {
	logFile, err := os.OpenFile("app"+strconv.Itoa(int(c.Client_id))+".log", os.O_CREATE|os.O_WRONLY, 0666)
	defer logFile.Close()
	// 设置日志输出到文件
	log.SetOutput(logFile)

	// 连接网络
	c.Stub = CreateClientStub(c.OnMsg)
	c.Stub.ConnectSyncTo(c.RemoteAddr)
	defer c.Stub.Close()
	// 发送首帧
	connFrame := &pb.ClientFirstFrame{
		Position: &pb.Position{
			ClientId: c.Client_id,
			X:        c.Position.X,
			Y:        c.Position.Y,
		},
	}
	dataConnFrame, err := proto.Marshal(connFrame)
	if err != nil {
		log.Fatalf("序列化失败: %v\n", err)
		return
	}
	connFrameMsg := message.CreateMessage(message.CONNECTFRAME, uint32(len(dataConnFrame)), 0, dataConnFrame)
	c.Stub.SendMsg(connFrameMsg)

	// 初始化 termbox
	if err := termbox.Init(); err != nil {
		log.Println("Failed to initialize termbox:", err)
		os.Exit(1)
	}
	defer termbox.Close()

	go c.GetCmd()

	uploadTicker := time.NewTicker(time.Duration(c.UpLoadTime) * time.Millisecond)
	renderTicker := time.NewTicker(time.Duration(c.RenderTime) * time.Millisecond)
	defer uploadTicker.Stop()
	defer renderTicker.Stop()
	for {
		select {
		case event := <-c.EventChan:
			// 处理键盘输入
			if event == byte(termbox.KeyEsc) {
				log.Printf("Catch esc, isReady<%v>\n", c.isReady)
				// 发送退出帧
				if !c.isReady {
					return
				}

				quitFrame := &pb.QuitFrame{
					ClientId: c.Client_id,
				}
				dataQuitFrame, err := proto.Marshal(quitFrame)
				if err != nil {
					log.Fatalf("序列化失败: %v\n", err)
					return
				}

				quitFrameMsg := message.CreateMessage(message.QUITFRAME, uint32(len(dataQuitFrame)), 0, dataQuitFrame)
				c.Stub.SendMsg(quitFrameMsg)
				c.Stub.Close()
				log.Printf("Client<%d> send ctrl frame<%v> to server\n", c.Client_id, quitFrame)
				return
			}

			if !c.isReady {
				continue
			}

			c.Position = c.PhysicalModel(string(event), c.Position)

		case <-renderTicker.C:
			// 绘图
			c.Render()

		case <-uploadTicker.C:
			// 上传操作
			c.SendCtrlCmdFrame()
		}
	}
}

func (c *Client) SendCtrlCmdFrame() {
	if !c.isReady {
		return
	}

	c.Lock.Lock()
	defer c.Lock.Unlock()

	c.secondFrameId++
	ctrlFrame := &pb.CtrlCmd{
		ClientId: c.Client_id,
		FirstId:  c.firstFrameId,
		SecondId: c.secondFrameId,
	}

	for c.Cmd.Len() > 0 {
		ch := c.Cmd.PopFront()
		ctrlFrame.Cmd += string(ch)
	}

	dataCtrlFrame, err := proto.Marshal(ctrlFrame)
	if err != nil {
		log.Fatalf("序列化失败: %v\n", err)
		return
	}

	ctrlFrameMsg := message.CreateMessage(message.UPLOADFRAME, uint32(len(dataCtrlFrame)), 0, dataCtrlFrame)

	c.Stub.SendMsg(ctrlFrameMsg)

	if len(ctrlFrame.Cmd) > 0 {
		log.Printf("Client<%d> send ctrl frame<%v> to server\n", c.Client_id, ctrlFrame)
	}
}

func (c *Client) Update(frame *pb.DataFrame) {
	clientCmds := frame.ClientCmds

	for _, clientCmd := range clientCmds {
		id := clientCmd.ClientId

		if id == c.Client_id {
			// 回滚状态
			for _, cmd := range clientCmd.Cmds {
				if cmd == nil || len(cmd.Cmd) == 0 {
					continue
				}

				tempPos := c.PhysicalModel(cmd.Cmd, c.lastPosition)

				// 更新最新一帧的 position
				c.lastPosition.X = tempPos.X
				c.lastPosition.Y = tempPos.Y

				for i := 0; i < c.Cmd.Len(); i++ {
					tempPos = c.PhysicalModel(string(c.Cmd.At(i)), tempPos)
				}

				if tempPos.X != c.Position.X || tempPos.Y != c.Position.Y {
					c.Position.X = tempPos.X
					c.Position.Y = tempPos.Y
				}
			}
			continue
		}

		for _, r := range c.Others {
			if r.ClientId == id {
				// 更新位置

				for _, cmd := range clientCmd.Cmds {
					if cmd == nil || len(cmd.Cmd) == 0 {
						continue
					}

					tempPos := c.PhysicalModel(cmd.Cmd, r)
					r.X = tempPos.X
					r.Y = tempPos.Y
				}
				break
			}
		}
	}
}
