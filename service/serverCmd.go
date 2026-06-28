package service

import (
	"fmt"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"net"
	"time"
)

type ServerCmdService struct{}

// List
func (is *ServerCmdService) List(page, pageSize uint) (res *model.ServerCmdList) {
	res = &model.ServerCmdList{}
	if page == 0 {
		page = 1
	}
	if pageSize == 0 {
		pageSize = 10
	}
	res.Page = int64(page)
	res.PageSize = int64(pageSize)

	commands := make([]*model.ServerCmd, 0, len(model.SysIdServerCmds)+len(model.SysRelayServerCmds))
	commands = append(commands, model.SysIdServerCmds...)
	commands = append(commands, model.SysRelayServerCmds...)
	tx := DB.Model(&model.ServerCmd{})
	var customCommands []*model.ServerCmd
	tx.Order("id ASC").Find(&customCommands)
	commands = append(commands, customCommands...)
	res.Total = int64(len(commands))
	start := int((page - 1) * pageSize)
	if start >= len(commands) {
		res.ServerCmds = []*model.ServerCmd{}
		return
	}
	end := start + int(pageSize)
	if end > len(commands) {
		end = len(commands)
	}
	res.ServerCmds = commands[start:end]
	return
}

// Info
func (is *ServerCmdService) Info(id uint) *model.ServerCmd {
	u := &model.ServerCmd{}
	DB.Where("id = ?", id).First(u)
	return u
}

// Delete
func (is *ServerCmdService) Delete(u *model.ServerCmd) error {
	return DB.Delete(u).Error
}

// Create
func (is *ServerCmdService) Create(u *model.ServerCmd) error {
	res := DB.Create(u).Error
	return res
}

// SendCmd 发送命令
func (is *ServerCmdService) SendCmd(port int, cmd string, arg string) (string, error) {
	//组装命令
	cmd = cmd + " " + arg
	res, err := is.SendSocketCmd("v6", port, cmd)
	if err == nil {
		return res, nil
	}
	//v6连接失败，尝试v4
	res, err = is.SendSocketCmd("v4", port, cmd)
	if err == nil {
		return res, nil
	}
	return "", err
}

// SendSocketCmd
func (is *ServerCmdService) SendSocketCmd(ty string, port int, cmd string) (string, error) {
	addr := "[::1]"
	tcp := "tcp6"
	if ty == "v4" {
		tcp = "tcp"
		addr = "127.0.0.1"
	}
	conn, err := net.Dial(tcp, fmt.Sprintf("%s:%v", addr, port))
	if err != nil {
		Logger.Debugf("%s connect to id server failed: %v", ty, err)
		return "", err
	}
	defer conn.Close()
	//发送命令
	_, err = conn.Write([]byte(cmd))
	if err != nil {
		Logger.Debugf("%s send cmd failed: %v", ty, err)
		return "", err
	}
	time.Sleep(100 * time.Millisecond)
	//读取返回
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil && err.Error() != "EOF" {
		Logger.Debugf("%s read response failed: %v", ty, err)
		return "", err
	}
	return string(buf[:n]), nil
}

func (is *ServerCmdService) Update(f *model.ServerCmd) error {
	return DB.Model(f).Updates(f).Error
}
