package main

import (
	"bufio"
	mtl "chat_server/src/mytools"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"strings"
	"time"
)

const secret_key = "redh3t_OnTheWayy"
const ARGUMENT_INDEX_NOT_FOUND = -1
const LOCK = true

var lock_channel = make(chan bool, 1)

// TODO 正式部署记得改密钥，不然github源码审计

const data_dir = "D:\\GolandProjects\\chat_server\\data\\"

func main() {
	listener, err := startListen(":5000")
	if err != nil {
		panic(err)
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println(err)
		}
		go handelConn(conn)
	}
}

func startListen(port string) (listener *net.TCPListener, err error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp4", port)
	if err != nil {
		return nil, err
	}
	listener, err = net.ListenTCP("tcp4", tcpAddr)
	if err != nil {
		return nil, err
	}
	return listener, nil
}

func handelConn(conn net.Conn) {
	defer func() {
		if recover() != nil {
			fmt.Println("a conn crashed")
		}
	}() // 并发中一个connection crash捕获panic
	fmt.Println("new handle_conn created")
	conn.SetReadDeadline(time.Now().Add(10 * time.Minute)) // 2 min time out
	defer conn.Close()

	for true {
		conn.SetReadDeadline(time.Now().Add(10 * time.Minute)) // 刷新断开连接时间
		cmd_str := getCmdString(conn)
		cmd_slice := processCmdStrToSlice(cmd_str)
		processCmd(conn, cmd_slice, cmd_str)
		fmt.Println("[recv from client]", cmd_str) // TODO log
	}
}

func getCmdString(conn net.Conn) (cmd_str string) {
	buff_size := 128
	var cmd_buff []byte
	for true { // 缓冲区
		buff := make([]byte, buff_size)
		length, err := conn.Read(buff)
		if err != nil {
			panic(err)
		}
		for _, v := range buff {
			if v != 0 {
				cmd_buff = append(cmd_buff, v)
			}
		}
		if length < buff_size {
			break
		}
	}
	// 很奇怪，不处理的时候[]byte会有0，客户端打空格的话cmd_slice会计数+1，而不打空格是正常的
	// 但是我寻思这和空格也没关系啊，空格是32又不是0，go总会有这种奇奇怪怪的问题
	// 这里中文没有处理，比如传入"啊"会存储为[229 149 138]
	cmd_str = string(cmd_buff)
	return cmd_str
}

func processCmdStrToSlice(cmd string) (cmd_slice []string) {
	command := bufio.NewScanner(strings.NewReader(cmd))
	command.Split(bufio.ScanWords)
	for command.Scan() {
		cmd_slice = append(cmd_slice, command.Text())
	}
	return cmd_slice
}

func processCmd(conn net.Conn, cmd []string, cmd_str string) (err error) {
	// 只传入空格报错的原因找到了！是因为cmd_slice根本没有[0]，直接越界了
	//fmt.Println(len(cmd))
	if len(cmd) == 0 {
		conn.Write([]byte("Hey why you type so many spaces?"))
		return nil
	}
	switch cmd[0] {
	case "register":
		if len(cmd) != 3 {
			conn.Write([]byte(getUsage("register")))
		} else if cmd[1] != "-id" {
			conn.Write([]byte(getUsage("register")))
		} else { // command valid
			result, err := register(cmd[2])
			if err != nil {
				fmt.Println(err)
			} else {
				conn.Write([]byte(result))
				// result可以是成功主策划返回的token，也可以是注册失败告诉client的一条指令
			}
		}
	case "login": // TODO 更新在线状态
		if len(cmd) != 3 {
			conn.Write([]byte(getUsage("login")))
		} else if cmd[1] != "-t" {
			conn.Write([]byte(getUsage("login")))
		} else {
			user_id, err := loginCheck(cmd[2])
			if err != nil {
				conn.Write([]byte("Token invalid, login failed:("))
			} else {
				conn.Write([]byte("[#clientmov#] login_success -id " + user_id))
			}
		}
	case "whoami":
		for k, v := range cmd {
			if v == "-id" && len(cmd) >= k+2 {
				conn.Write([]byte("you login as : " + cmd[k+1]))
				break
			}
		}
	case "sendmsg":
		to_index := ARGUMENT_INDEX_NOT_FOUND
		msg_index := ARGUMENT_INDEX_NOT_FOUND
		id_index := ARGUMENT_INDEX_NOT_FOUND
		for k, v := range cmd {
			if v == "-to" {
				to_index = k
			} else if v == "-msg" {
				msg_index = k
			} else if v == "-id" {
				id_index = k
			}
		}
		if to_index == ARGUMENT_INDEX_NOT_FOUND || msg_index == ARGUMENT_INDEX_NOT_FOUND || id_index == ARGUMENT_INDEX_NOT_FOUND {
			conn.Write([]byte(getUsage("sendmsg")))
			return nil
		} else if to_index > msg_index {
			conn.Write([]byte(getUsage("sendmsg")))
			return nil
		}
		// 先正则匹配一部分算了，需要原先的str
		match_msg := regexp.MustCompile("-msg .* -id ")
		// TODO 非贪婪or贪婪？
		msg_str := match_msg.FindString(cmd_str)
		msg_str = msg_str[5 : len(msg_str)-5]
		recv_id := cmd[to_index+1]
		_, err = os.Stat(data_dir + recv_id + ".chat")
		if err != nil {
			// 文件不存在，也就是id invalid
			conn.Write([]byte("sorry, currently we don't have a user named " + recv_id + " :("))
			return nil
		}
		send_id := cmd[id_index+1]
		str_in_send := "[send]{{{to::" + recv_id + "}}}{{{content::" + msg_str + "}}}\n"
		str_in_recv := "[recv]{{{from::" + send_id + "}}}{{{content::" + msg_str + "}}}[unchecked]\n"
		// lock
		lock_channel <- LOCK
		send_file_path := data_dir + send_id + ".chat"
		file, _ := os.OpenFile(send_file_path, os.O_WRONLY|os.O_APPEND, 0666)
		file.WriteString(str_in_send)
		file.Close()
		<-lock_channel       // unlock
		lock_channel <- LOCK // lock again
		recv_file_path := data_dir + recv_id + ".chat"
		file, _ = os.OpenFile(recv_file_path, os.O_WRONLY|os.O_APPEND, 0666)
		file.WriteString(str_in_recv)
		file.Close()
		<-lock_channel // unlock
		conn.Write([]byte("done"))
	case "checkmsg":
		fmt.Println(len(cmd))
		if len(cmd) != 3 && len(cmd) != 4 {
			conn.Write([]byte(getUsage("checkmsg")))
			return nil
		}
		id_index := ARGUMENT_INDEX_NOT_FOUND
		all_index := ARGUMENT_INDEX_NOT_FOUND
		for k, v := range cmd {
			if v == "-id" {
				id_index = k
			} else if v == "-all" {
				all_index = k
			}
		}
		line_slice := make([]string, 0)
		lock_channel <- LOCK
		f, _ := os.Open(data_dir + cmd[id_index+1] + ".chat")
		r := bufio.NewReader(f)
		for true {
			line_byte, _, err := r.ReadLine()
			if err == io.EOF {
				break
			}
			line_str := string(line_byte)
			line_slice = append(line_slice, line_str)
		}
		f.Close()
		<-lock_channel
		send_client_slice := make([]string, 0)
		for k, v := range line_slice {
			if k == 0 { // 第一行是注册信息
				continue
			}
			if v[:6] != "[recv]" {
				continue
			}
			match_msg := regexp.MustCompile("(?U){{{content::.*}}}")
			msg := match_msg.FindString(v)[12:]
			msg = msg[:len(msg)-3]
			match_from := regexp.MustCompile("(?U){{{from::.*}}}")
			from_id := match_from.FindString(v)[9:]
			from_id = from_id[:len(from_id)-3]
			unchecked_flag := false
			if v[len(v)-11:] == "[unchecked]" {
				unchecked_flag = true
			}
			if (all_index == ARGUMENT_INDEX_NOT_FOUND && unchecked_flag == true) || all_index != ARGUMENT_INDEX_NOT_FOUND {
				send_client_slice = append(send_client_slice, "From @"+from_id+": "+msg)
			}
		}

		if len(send_client_slice) == 0 {
			conn.Write([]byte("No message"))
			return nil
		}
		send_client_str := ""
		for k, v := range send_client_slice {
			if k == 0 {
				send_client_str += v
			} else {
				send_client_str += "\n" + v
			}
		}
		conn.Write([]byte(send_client_str))

		// update states check -all will not influence
		if all_index == ARGUMENT_INDEX_NOT_FOUND {
			lock_channel <- LOCK
			// 写一个tmp文件，之后删除源文件 重命名新文件
			tmp_file, _ := os.Create(data_dir + cmd[id_index+1] + ".chat_tmp")
			f, _ := os.Open(data_dir + cmd[id_index+1] + ".chat")
			r := bufio.NewReader(f)
			for true {
				line_byte, _, err := r.ReadLine()
				if err == io.EOF {
					break
				}
				line_str := string(line_byte)
				if line_str[len(line_str)-11:] == "[unchecked]" {
					tmp_file.Write([]byte(line_str[:len(line_str)-11] + "\n"))
				} else {
					tmp_file.Write([]byte(line_str + "\n"))
				}
			}
			f.Close()
			tmp_file.Close()
			os.Remove(data_dir + cmd[id_index+1] + ".chat")
			os.Rename(data_dir+cmd[id_index+1]+".chat_tmp", data_dir+cmd[id_index+1]+".chat")
			<-lock_channel
		}
		return nil
	default:
		conn.Write([]byte("Unknown command"))
	}
	return nil
}

func getUsage(cmd string) (usage string) {
	switch cmd {
	case "register":
		return "[usage] register -id (your_id_here_no_space_allowed)"
	case "login":
		return "[usage] login -t (your_token)"
	case "whoami":
		return "[usage] whoami"
	case "sendmsg":
		return "[usage] sendmsg -to (receiver_id) -msg your_message here\n[warning] -msg must be the last arg"
	case "checkmsg":
		return "[usage] checkmsg [-all]"
	default:
		return "command " + cmd + " not found"
	}
}

func register(id string) (result string, err error) {
	data_file_path := data_dir + id + ".chat"
	_, err = os.Stat(data_file_path)
	if err != nil {
		// chat文件不存在，允许注册
		file, err := os.Create(data_file_path)
		if err != nil {
			fmt.Println(err)
		} else {
			_, err = file.Write([]byte("[created] id: " + id + " time: " + time.Now().Format("2006-01-02 15:04:05") + "\n"))
			file.Close()
		}
		encrypted, err := mtl.AesEncrypt([]byte(id), []byte(secret_key))
		if err != nil {
			return "", err
		}
		result = base64.StdEncoding.EncodeToString(encrypted)
		result = "Register success, your token here : " + result
		return result, nil
	} else {
		return "ID already exists", nil
	}

}

func loginCheck(token_str string) (user_id string, err error) {
	tmp, err := base64.StdEncoding.DecodeString(token_str)
	if err != nil {
		return "", err
	}
	tmp, err = mtl.AesDecrypt(tmp, []byte(secret_key))
	// aes用的是别人的代码，不晓得会有panic，只能用recover了
	r := recover()
	if r != nil { // panic了
		return "", errors.New("AesDecrypt crashed")
	} else {
		return string(tmp), nil
	}

}
