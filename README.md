# Chat Official

## 前言

这个项目起因是车带暑期学校比较空闲，不想整天打游戏浪费时间，另外一个想法是想通过一个项目入门golang。最开始我考虑的是实现一个HTTP服务器，后来想Flask已经实现过一个了，想想就算了（虽然go也有SSTI但是和Python的大同小异，不是很有兴趣）。以前一直想做一个聊天程序，这个暑假我又刚好拥有了个人VPS，既然这样，那就实现一个基于Socket的chat程序吧

## Chat Official 项目构成

* 服务端 chat_server
* 客户端 chat_client

## 客户端开放指令

* register
* login
* whoami
* sendmsg
* checkmsg
* startchat (已放弃开发)
* help

具体用法示例

```
@[?]>>> help
(content...)
@[?]>>> help whoami
[usage] whoami
@[?]>>> 
```

## 开发过程记录

### 0x00 身份认证

其实之前在打CTF的时候有些平台的战队就算直接给token，我在想直接免密登录就可以，一开始的想法是注册的时候给定id，服务端生成并返回一个token，下次登录的时候用户给定id和token，服务端比对根据id生成的token和给定的token是否一直来判断是否合法登录，之后想到能不能省略提供id，我又想到了非对称加密，之后发现可能没有必要，我本身不需要公开公钥，因此直接用对称加密就行。因此最终采用的方案是AES对称加密

token的好处是服务端不需要存储密码，只需要保存算法以及密钥即可。以下是一些现成的token，我拿它们来做测试

```
redh3t token：
CzQ2raKAMcoJDORbFrd6rg==

debug token：
UXgWzbdnk9YW3dcvLHACSQ==

test token：
YbG/kLan2yPH2k5Zsm0gBA==
```

### 0x01 交互

客户端并不是无脑接受消息并且打印在屏幕上，一旦受到特定字符串，可以执行一些特定的操作。也就是执行来自server的指令。这类指令全部以[#clientmov#]开头。可以在服务器需要的时候do something

### 0x02 数据存储

在我搜索go orm之前我一直以为golang数据库使用很方便...

***支线任务开启：自己造轮子写一个 go orm，要求不高，只需要提供对 sqlite的支持就行，我决定取名为 segolite***

回归主线任务。放弃难用的orm以及数据库，采用文件存储的方式。我一开始想的是采用json，后来不知道为什么，大概是忘记了吧，就还是采用之前一直用的存储模式：特定格式+正则匹配。别问，我可能是正则的狂热爱好者（雾）

每个账户对应一个数据文件如id=redh3t对应redh3t.chat，注册时自动生成，并添加第一行文字"redh3t created at[time]"，之后按顺序存储receive和sendto记录（本来记录来源和去向以及时间想采用每一行都是一个json格式字符串的解决办法）客户端sendmsg命令会同时更新两个文件：自己的和对方的

并发下的读写就需要涉及到锁了，我一开始是想维护一个全局map：make(map[string]chan bool)，一个文件对应一把锁，这样要读写某个文件先根据文件名拿到锁，根据锁的状态来决定读写。这很好，但是很可惜，golang原生的map不是并发安全的，并发安全的map在另外一个包里面。我没有精力学，因此就想到再开一把调度map的锁，但是显然一点也不优雅。最后我直接一刀切，这个server运行的时候单次只允许操作一个文件，全局只设置一把文件锁，就算要操作的不是当前正在操作的文件也得阻塞排队。

### 0x03 在线状态更新

一个用户login成功之后需要记录为在线，更换账户登录之后，原账户需要下线，新账户需要上线，在connection crash的时候原账户需要自动下线。在需要开启实时聊天的时候需要检测对方是否在线。我采用的是在server端维护一个全局变量叫online_id，它是一个[]string，里面存储的是所有已上线的用户id。为什么是string而不是conn，我的理解是一个conn对应的id会变化，这样子就不太方便。

用[]string维护天然支持多终端在线，而且还能统计多少个终端在线，虽然我没写这个功能但是我觉得可以实现一下

后来我重构了一下，现在全局又多了一个[]SESSION，一个SESSION存了conn id等信息

如何在直接强制关闭Terminal（connection crash）的情况下检测到是哪一个账户并直接offline呢？解决办法就是在conn close的defer里面添加offline函数，直接recover()捕获panic并让其下线。

### 0x04 客户端的处理

客户端应该是需要并发的，一个用于输出，一个用于输入，接受server的应该是要副线程，处理输入并发送数据的应该是主线程。之前写的逻辑是先获取用户输入，再发送数据，再监听server直到收到回复，再处理回复，然后循环上述过程。显然这在之前的业务逻辑是没有问题的，但是一旦来到了实时聊天功能，这个就不行了。因为发起实时聊天之后需要实现对方在尚未向server发送消息的情况下收到来自server的闻讯消息

### 0x05 in-time chat

https://docs.hacknode.org/gopl-zh/ch8/ch8-10.html

理想的情况是client发送startchat命令，服务器收到之后，向对方的那个conn发送询问消息

例程有个函数是交出当前cpu使用权

那我就再上个锁，平时一直锁着，当检测到startchat指令的时候解锁，然后执行权交给专门处理startchat的goroutine。当然不能是全局锁，所以应该是SESSION struct里面的一个属性

感觉这样可扩展啊，还可以再做一个global chat的东西

直接用锁的阻塞是不行的，因为goroutine不一定有机会运行，所以要有Goexit

但是goroutine运行时间过长会发生抢占调度，也就是说chat的goroutine太久了会造成main直接开始运行解决办法是当前的handleConn协程发起handleIntimeChat然后自己直接退出，在handleIntimeChat结束的时候再开handleConn，直接再defer里面就行

直接退出当前的goroutine会造成和client的连接丢失！不可取！

不折腾了，直接再switch里面写算了

提示符直接改为服务端来显示吧

这样intime chat是需要的调整就能简单一点

记得加一堆的\n

为什么第一次打y/n是unknowncommand，第二次才是正确的反应：

被请求的一方面之前printPromote之后就是处于接受command，先走的是正常的那个getCmdString，之后才轮到接受询问结果的getCmdString，办法是加上clientmov，让client send一条指令叫noresopnse，server新增一条noresponse指令，什么也不做

发起的一方是正常的，接受的那一方由于server getCmdString是在正常mode和chatmode里面交替，就会出现一半正常一半不正常，chan锁不知道能不能解决  可以解决，再加上一个send noresponse就行

获取chat是否同意的y/n是排队的，排在一个getCmdString后面，因此需要client先发送一点东西把前面这个接收消息的通道用掉，也就是send_noresponse

写即时聊天颅内编译真的很容易断片过载，就为了调那几个顺序，，比如把锁的代码往上移几行就能完美解决问题，，，淦

### 0xFE 问题碎碎念（属于是看不懂自己早期的随手笔记了）

很奇怪，之前还是可以的现在是只打空格或者单独一个回车就直接阻塞，不晓得为什么。。论版本管理的重要性

但是单独处理空格那里我发现如果返回的不是空字符串那么空格的情况就不会阻塞，有可能是发送空字符串的时候会造成接收端的阻塞！

client单独打一个回车，send数据长度是0，也就是说不存在实际send的数据，然后client进入recv阻塞，而服务端没有受到send来的数据，服务端也一直卡在recv，造成client等serv回应，serv等client数据的死锁

要是client判断到什么东西不需要向服务端发送数据的时候，sendcmd里面建议处理完之后直接return errors.New("empty_send")

processCmd里面的一些注释
// 只传入空格报错的原因找到了！是因为cmd_slice根本没有[0]，直接越界了
// 处理uncheck的办法是重新写一个tmp文件，之后删除源文件，再重命名tmp文件

重构完了删掉-id忘记改正则了哈哈哈哈哈，正则不对匹配不到东西

突然想到github commit message会不会有xss啊（笑）

### 0xFF

2022.9.3

遇到了一个很可爱的女孩子，傍晚和她一起出去玩，心要化了，这个项目已经开发不下去了，没这个心思了，接下来几天应该要好好地缓一缓。另外就要开学了，以后应该也没这个时间来继续开发它了，这个项目估计就要夭折了吧。
