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

// TODO 正式部署记得改密钥，不然github源码审计

const data_dir = "D:\\GolandProjects\\chat_server\\data\\"
const secret_key = "redh3t_OnTheWayy" //kfccrazythursdayVme50 21bit long
const NOT_LOG_IN = "youjustdidntlogin"
const INTIME_CHAT_NO_OTHER_ID = "intime_chat_no_other_id"
const (
	ARGUMENT_INDEX_NOT_FOUND = -iota
	NO_LOGIN_INDEX
	INDEX_TO_OFFLINE_NOT_FOUND
)

var lock_channel = make(chan bool, 1) // file read|write lock
var online_id = make([]string, 0)

type SESSION struct { // TODO 不知道还要有什么
	conn                     net.Conn
	curr_id                  string
	intime_chat_the_other_id string
	intime_chat_lock         chan bool
}
type CMD struct {
	cmd_str   string
	cmd_slice []string
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
		user_id = string(tmp)
		return user_id, nil
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
func getUsage(cmd string) (usage string) {
	switch cmd {
	case "register":
		return "[usage] register (your_id_here_no_space_allowed)\n"
	case "login":
		return "[usage] login (your_token)\n"
	case "whoami":
		return "[usage] whoami\n"
	case "sendmsg":
		return "[usage] sendmsg -to (receiver_id) -msg your_message here\n[warning] -msg must be the last arg\n"
	case "checkmsg":
		return "[usage] checkmsg [-all]\n"
	case "startchat":
		return "[usage] startchat -with (id)\n"
	case "help":
		return "currently we support the following commands:\nregister\nlogin\nwhoami\nsendmsg\ncheckmsg\nstartchat(still under development...)\nto see any of them ,type 'help (cmd)'\n"
	default:
		return "command " + cmd + " not found, type 'help' for more info\n"
	}
}
func printOnline(online_slice []string) {
	fmt.Println("[current online]")
	for _, v := range online_slice {
		fmt.Println(v)
	}
	fmt.Println("[online id printed]")
} // TODO debug
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

		new_intime_chat_lock := make(chan bool, 1)
		go handelConn(&SESSION{
			conn:                     conn,
			curr_id:                  NOT_LOG_IN,
			intime_chat_the_other_id: INTIME_CHAT_NO_OTHER_ID,
			intime_chat_lock:         new_intime_chat_lock,
		})
	}
}

func handelConn(sess *SESSION) { // 每次用go单开的一个线程都对应一个正在使用的client
	defer func() {
		if recover() != nil {
			if sess.curr_id == NOT_LOG_IN {
				fmt.Println("a sess.conn crashed, but it didn't even login")
			} else {
				// 复用了
				index_to_offline := NO_LOGIN_INDEX
				for k, v := range online_id {
					if v == sess.curr_id {
						index_to_offline = k
						break
					}
				}
				if index_to_offline == NO_LOGIN_INDEX {
					fmt.Println("Damn why I cannot find the id to make it offline!")
				} else {
					online_id = append(online_id[:index_to_offline], online_id[index_to_offline+1:]...)
				}
				// 复用结束
				fmt.Println("a sess.conn crashed, id " + sess.curr_id + " automatically offline")
			}
		}
	}() // 捕获conn crash的panic， 如果已登录则自动从online_id中删除
	defer sess.conn.Close()
	fmt.Println("in handleConn, current id: ", sess.curr_id) // TODO debug
	for true {
		sess.conn.SetReadDeadline(time.Now().Add(10 * time.Minute)) // 刷新断开连接时间
		sendNormalPrompt(sess)
		cmd_str := getCmdString(sess.conn)
		cmd_slice := processCmdStrToSlice(cmd_str)
		rewriteProcessCmd(sess, CMD{
			cmd_str:   cmd_str,
			cmd_slice: cmd_slice,
		})
	}
}

func rewriteProcessCmd(sess *SESSION, cmd CMD) { // 应该传指针，这样的话才能修改原来的那个session
	if len(cmd.cmd_slice) == 0 { // 解决client只打空格的问题
		sess.conn.Write([]byte("Hey why you type so many spaces?"))
	} else {
		switch cmd.cmd_slice[0] {
		case "whoam!":
			if len(cmd.cmd_slice) != 1 {
				sess.conn.Write([]byte(getUsage("whoami")))
			} else {
				if sess.curr_id == NOT_LOG_IN {
					sess.conn.Write([]byte("Careless Whispers by whoam!, that is my favorite song ever!!!!!!\nbut you are just a guest..."))
				} else {
					sess.conn.Write([]byte("I am saying 'hi' to @" + sess.curr_id + ", yeah I mean you, I must confess that\nCareless Whispers is my favorite song EVER!!!!!!"))
				}
			}
		case "whoami":
			if len(cmd.cmd_slice) != 1 {
				sess.conn.Write([]byte(getUsage("whoami")))
			} else {
				if sess.curr_id == NOT_LOG_IN {
					sess.conn.Write([]byte("you are just a guest..."))
				} else {
					sess.conn.Write([]byte("you login as @" + sess.curr_id))
				}
			}
		case "register":
			if len(cmd.cmd_slice) != 2 {
				sess.conn.Write([]byte("register"))
			} else {
				result, _ := register(cmd.cmd_slice[1])
				sess.conn.Write([]byte(result))
			} // 返回token或者id exists
		case "login":
			if len(cmd.cmd_slice) != 2 {
				sess.conn.Write([]byte(getUsage("login")))
			} else {
				user_id, err := loginCheck(cmd.cmd_slice[1])
				if err != nil {
					sess.conn.Write([]byte("Token invalid, login failed:("))
				} else {
					sess.conn.Write([]byte("[#clientmov#] login_success -id " + user_id))
					// 以下操作是更新server记录的在线信息
					if sess.curr_id != NOT_LOG_IN {
						// 已登录，需要先下线
						index_to_offline := INDEX_TO_OFFLINE_NOT_FOUND
						for k, v := range online_id {
							if v == sess.curr_id {
								index_to_offline = k
								break
							}
						}
						if index_to_offline == INDEX_TO_OFFLINE_NOT_FOUND {
							fmt.Println("Damn why I cannot find the id to make it offline!")
						} else {
							online_id = append(online_id[:index_to_offline], online_id[index_to_offline+1:]...)
						}
					}
					// 上线
					sess.curr_id = user_id
					online_id = append(online_id, sess.curr_id)
				}
			}
		case "sendmsg":
			to_index := ARGUMENT_INDEX_NOT_FOUND
			msg_index := ARGUMENT_INDEX_NOT_FOUND
			for k, v := range cmd.cmd_slice {
				if v == "-to" {
					to_index = k
				} else if v == "-msg" {
					msg_index = k
				}
			}
			if to_index == ARGUMENT_INDEX_NOT_FOUND || msg_index == ARGUMENT_INDEX_NOT_FOUND {
				sess.conn.Write([]byte(getUsage("sendmsg")))
			} else if to_index > msg_index {
				sess.conn.Write([]byte(getUsage("sendmsg")))
			} else { // 寻找各项参数结束，并完成check valid工作
				recv_id := cmd.cmd_slice[to_index+1]
				_, err := os.Stat(data_dir + recv_id + ".chat")
				if err != nil {
					sess.conn.Write([]byte("sorry, currently we don't have a user named " + recv_id + " :("))
				} else {
					msg_str := regexp.MustCompile("-msg .*").FindString(cmd.cmd_str)[5:]
					send_id := sess.curr_id
					time_stamp := time.Now().Format("2006-01-02 15:04:05")
					str_in_send := "[send]{{{to::" + recv_id + "}}}{{{content::" + msg_str + "}}}{{{timestamp::" + time_stamp + "}}}\n"
					str_in_recv := "[recv]{{{from::" + send_id + "}}}{{{content::" + msg_str + "}}}{{{timestamp::" + time_stamp + "}}}[unchecked]\n"
					lock_channel <- true // file lock
					file, _ := os.OpenFile(data_dir+send_id+".chat", os.O_WRONLY|os.O_APPEND, 0666)
					file.WriteString(str_in_send)
					file.Close()
					<-lock_channel       // unlock
					lock_channel <- true // lock again
					file, _ = os.OpenFile(data_dir+recv_id+".chat", os.O_WRONLY|os.O_APPEND, 0666)
					file.WriteString(str_in_recv)
					file.Close()
					<-lock_channel // unlock
					sess.conn.Write([]byte("done"))
				}
			}
		case "checkmsg":
			if len(cmd.cmd_slice) != 1 && len(cmd.cmd_slice) != 2 {
				sess.conn.Write([]byte(getUsage("checkmsg")))
			} else {
				all_index := ARGUMENT_INDEX_NOT_FOUND
				if len(cmd.cmd_slice) == 2 {
					if cmd.cmd_slice[1] == "-all" {
						all_index = 1
					} else {
						sess.conn.Write([]byte(getUsage("checkmsg")))
						break
					}
				} // 检查是否有-all参数
				line_slice := make([]string, 0)
				lock_channel <- true // file lock
				f, _ := os.Open(data_dir + sess.curr_id + ".chat")
				r := bufio.NewReader(f)
				for true {
					line_byte, _, err := r.ReadLine()
					if err == io.EOF {
						break
					}
					line_str := string(line_byte)
					line_slice = append(line_slice, line_str)
				} // 读取文件每一行的内容
				f.Close()
				<-lock_channel // unlock
				send_client_slice := make([]string, 0)
				for k, v := range line_slice {
					if k == 0 { // 第一行是注册信息
						continue
					}
					if v[:6] != "[recv]" {
						continue
					}
					msg := regexp.MustCompile("(?U){{{content::.*}}}").FindString(v)[12:]
					msg = msg[:len(msg)-3]
					from_id := regexp.MustCompile("(?U){{{from::.*}}}").FindString(v)[9:]
					from_id = from_id[:len(from_id)-3]
					if (all_index == ARGUMENT_INDEX_NOT_FOUND && v[len(v)-11:] == "[unchecked]") || all_index != ARGUMENT_INDEX_NOT_FOUND {
						send_client_slice = append(send_client_slice, "From @"+from_id+": "+msg)
					}
				} // 组装输出内容
				if all_index == ARGUMENT_INDEX_NOT_FOUND {
					lock_channel <- true
					tmp_file, _ := os.Create(data_dir + sess.curr_id + ".chat_tmp")
					f, _ := os.Open(data_dir + sess.curr_id + ".chat")
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
					os.Remove(data_dir + sess.curr_id + ".chat")
					os.Rename(data_dir+sess.curr_id+".chat_tmp", data_dir+sess.curr_id+".chat")
					<-lock_channel
				} // check不带-all将会更新是否已阅的标记
				if len(send_client_slice) == 0 {
					sess.conn.Write([]byte("no new message received"))
				} else {
					send_client_str := ""
					for k, v := range send_client_slice {
						if k == 0 {
							send_client_str += v
						} else {
							send_client_str += "\n" + v
						}
					}
					sess.conn.Write([]byte(send_client_str))
				} // 输出到client环节
			}
		case "help":
			if len(cmd.cmd_slice) != 2 {
				sess.conn.Write([]byte(getUsage("help")))
			} else {
				sess.conn.Write([]byte(getUsage(cmd.cmd_slice[1])))
			}
		case "printonline": // TODO debug cmd
			printOnline(online_id)
		case "startchat":
			handleIntimeChat(sess)
		default:
			sess.conn.Write([]byte("unknown command, type 'help' to see more\n"))
		}
	}
}

func handleIntimeChat(sess *SESSION) { // TODO 下面是测试代码，成功！！！
	fmt.Println("in handleIntimeChat func")
	for true {
		sess.conn.Write([]byte("[in-time chat] @[" + sess.curr_id + "]#> "))
		intime_chat_recv_str := getCmdString(sess.conn) // 虽然名字会感觉有歧义，但是复用就完了！
		sess.conn.Write([]byte("your type: " + intime_chat_recv_str))
		if intime_chat_recv_str == "exit" {
			break
		}
	}
}

func sendNormalPrompt(sess *SESSION) {
	if sess.curr_id == NOT_LOG_IN {
		sess.conn.Write([]byte("@[?]>>> "))
	} else {
		sess.conn.Write([]byte("@[" + sess.curr_id + "]>>> "))
	}
}
