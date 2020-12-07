package model

import (
	"database/sql"
	"errors"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gomodule/redigo/redis"
	"log"
	"strconv"
	"sync"
	"time"
)

const (
	FREE    = 0
	NOTSIGN = 1
	USING   = 2
)

var mu sync.Mutex

var ID = 0 // 唯一ID

var Mark map[string]int // key是ROOMID+STARTTIME value是唯一ID

var pool *redis.Pool //创建redis连接池
var db *sql.DB       // mysql

type Channel struct {
	Read      chan struct{}
	Write     chan struct{}
	isWriting bool
	isReading int
}

var RWChannel []Channel

type Data struct {
	RoomId         string
	UserName       string
	UserPhone      string
	UserEmail      string
	StartTime      string
	UserRemark     string
	MeetingTopic   string
	UserDepartment string
}

func NewData() *Data {
	return &Data{
		RoomId:         "",
		UserName:       "",
		UserPhone:      "",
		UserEmail:      "",
		StartTime:      "",
		UserRemark:     "",
		MeetingTopic:   "",
		UserDepartment: "",
	}
}

func NewChannel() *Channel {
	return &Channel{
		Read:      make(chan struct{}, 1000), // 最多允许1000个并发读
		Write:     make(chan struct{}, 1),    // 只允许一个请求读
		isWriting: false,
		isReading: 0,
	}
}

func init() {
	// redis pool
	pool = &redis.Pool{ //实例化一个连接池
		MaxIdle:     16,  //最初的连接数量
		MaxActive:   0,   //连接池最大连接数量,不确定可以用0（0表示自动定义），按需分配
		IdleTimeout: 300, //连接关闭时间 300秒 （300秒不使用自动关闭）
		Dial: func() (redis.Conn, error) { //要连接的redis数据库
			return redis.Dial("tcp", "127.0.0.1:6379")
		},
	}

	// mysql
	db, _ = sql.Open("mysql", )

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	for i := 0; i < 1000; i++ { // 设置默认值
		RWChannel = append(RWChannel, *NewChannel())
	}
}

// 输入roomid+starttime
func getMark(str string) int {
	res, ok := Mark[str]
	if !ok {
		return setMark(str)
	}
	return res
}

// input roomid+starttime
func setMark(str string) int {
	mu.Lock()

	res, ok := Mark[str]
	if ok {
		mu.Unlock()
		return res
	}
	ID++
	Mark[str] = ID

	if ID > len(RWChannel) { // 当分配的ID超过数组大小时要设置好默认值,按照2倍提前扩容
		templen := len(RWChannel)
		for i := templen; i <= ID*2; i++ {
			RWChannel = append(RWChannel, *NewChannel())
		}
	}

	mu.Unlock()
	return ID
}

// 验证用户数据信息和会议室预约人是否一致
// input ：用户名，用户部门， [notSign | Sign], 要查询的会议室名+使用时间
func judgePermissions(username, userdepartment, key string) bool {

	tempid := getMark(key)

	for i := 0; i < 100; i++ {
		select {
		case RWChannel[tempid].Read <- struct{} {} :
			if RWChannel[tempid].isWriting == true {
				<- RWChannel[tempid].Read
				time.Sleep(30 * time.Millisecond)
				continue
			}
			mu.Lock()
			RWChannel[tempid].isReading++
			mu.Unlock()

			c := pool.Get()
			defer func () {
				err := c.Close()
				if err != nil {
					log.Println("close redis failed")
				}
				mu.Lock()
				RWChannel[tempid].isReading--
				<- RWChannel[tempid].Read
				mu.Unlock()
			} ()

			value, err := redis.String(c.Do("get", key))
			if err != nil {
				log.Println("redis get failed")
			}
			user := userdepartment + username
			return value == user
		}
	}
	return false
}

// 查询所有空闲会议室
func QueryAllFreeRoom() ([]string, error) {
	// 向所有会议室全部isreading+1， 拿到所有权限保证不会被写。
	for i := 1; i <= ID; i++ {
		for j := 0; j < 30; j++ {
			select {
			case RWChannel[i].Read <- struct {} {} :
				if RWChannel[i].isWriting == true {
					<- RWChannel[i].Read
					time.Sleep(10 * time.Millisecond)
					continue
				}
				mu.Lock()
				RWChannel[i].isReading++
				mu.Unlock()
			}
		}
	}

	c := pool.Get()
	defer func () {
		err := c.Close()
		if err != nil {
			log.Println(err)
		}
		// 释放掉所有权限
		for i := 1; i <= ID; i++ {
			mu.Lock()
			<- RWChannel[i].Read
			RWChannel[i].isReading--
			mu.Unlock()
		}
	} ()


	ret := make([]string, 0)

	script := "local ans, has, cursor = {}, {}, \"0\"\nrepeat\n    local t = redis.call(\"SCAN\", cursor)\n    local list = t[2]\n    for i = 1, #list do\n        local s = list[i]\n        if has[s] == nil then\n            has[s] = 1\n            local value = redis.call(\"get\", s)\n            if value == \"0\" then\n                ans[#ans + 1] = s\n            end\n        end\n    end\n    cursor = t[1]\nuntil cursor == \"0\"\nreturn ans --or return ans"
	arr, err := redis.Values(c.Do("eval", script, 0))
	if err != nil {
		return ret, errors.New(err.Error())
	}
	for _, v := range arr {
		mu.Lock()
		ret = append(ret, string(v.([]byte)))
		mu.Unlock()
	}
	return ret, nil
}


// value is value in redis key-value
func cacheData(tempid int, roomid, starttime, value string) { // 使用协程去写入redis，在读的情况下不能够写入。
	select {
	case RWChannel[tempid].Write <- struct{}{}:
		mu.Lock()
		if RWChannel[tempid].isWriting == true {
			<-RWChannel[tempid].Write
			mu.Unlock()
			return
		}
		for {
			if RWChannel[tempid].isReading == 0 {
				break
			}
		}
		RWChannel[tempid].isWriting = true
		mu.Unlock()

		c := pool.Get()
		defer func(temp int) {
			err := c.Close()
			if err != nil {
				log.Println("redis close error !")
			}
			RWChannel[temp].isWriting = false
			<-RWChannel[temp].Write
		}(tempid)

		if _, err := redis.Int(c.Do("set", roomid+starttime, value)); err != nil {
			log.Println(err)
		}
	default:
		return
	}
}

// 查询某时间段会议室状态
func QueryRoomStatus(RoomId, StartTime string) (int, bool) {
	room := RoomId + StartTime
	tempid := getMark(room)

	for i := 0; i < 50; i++ {
		select {
		case RWChannel[tempid].Read <- struct{}{}: // 获取令牌，限制最多1000个读的请求
			if RWChannel[tempid].isWriting == true { // 读的前提是不能正在写入
				<-RWChannel[tempid].Read
				time.Sleep(time.Millisecond * 20)
				continue
			}
			mu.Lock()
			RWChannel[tempid].isReading++ // 获得允许，可以进行读
			mu.Unlock()
			c := pool.Get()
			defer func() {
				err := c.Close()
				if err != nil {
					log.Println("close redis pool error. ", err)
				}
				mu.Lock()
				<-RWChannel[tempid].Read
				RWChannel[tempid].isReading--
				mu.Unlock()
			}()

			res, err := redis.String(c.Do("get", room)) // 首先从redis缓存中查询
			if err != nil {
				log.Println("查询redis失败，重试中")
				res, err = redis.String(c.Do("get", room))
				if err != nil {
					log.Println("查询redis再次失败")
					continue
				}
			}
			if res != "" {
				if res[0] == '0' {
					return FREE, true
				}
				if res[0] == '1' {
					return NOTSIGN, true
				}
				if res[0] == '2' {
					return USING, true
				}
			}
			retStatus := FREE
			// 从redis中没有找到, 再去mysql查询
			msql := "select name, department, status from userInfo where roomId = ? and meetingStartTime = ? limit 1"
			rows, err := db.Query(msql, RoomId, StartTime)
			if err != nil {
				log.Println(err)
			}
			name := ""
			department := ""
			for rows.Next() {
				var status string
				rows.Scan(&name, &department, &status)
				if status == "notSign" {
					retStatus = NOTSIGN
				} else if status == "Sign" {
					retStatus = USING
				}
			}

			val := strconv.Itoa(retStatus) + department + name
			// 查询之后存到redis缓存
			go cacheData(tempid, RoomId, StartTime, val)
			return retStatus, true // 强行保证defer函数一定能够执行，因为上述都没能够返回会导致 defer 资源泄漏
		default:
			continue
		}
	}
	return -1, false
}

// 预约会议室
// bookRoom  返回0表示正常 返回1 表示会议室已被占用
func BookRoom(data *Data) error {

	value := data.UserDepartment + data.UserName
	room := data.RoomId + data.StartTime

	NowStatus, ok := QueryRoomStatus(data.RoomId, data.StartTime)
	if !ok {
		return errors.New("查询会议室状态失败")
	}

	if NowStatus != FREE {
		log.Println("目标时间的该会议室已经被占用,预约失败")
		return errors.New("目标时间的该会议室已经被占用,预约失败")
	}

	tempid := getMark(room)

	select {
	case RWChannel[tempid].Write <- struct{}{}: 	// 没必要上锁，能进去的只有一个请求。
		if RWChannel[tempid].isWriting == true {
			<-RWChannel[tempid].Write
			mu.Unlock()
			return errors.New("服务器繁忙请重试 🐶")
		}
		RWChannel[tempid].isWriting = true
		for {
			if RWChannel[tempid].isReading == 0 {
				break
			}
			// 当前仍有请求正在读入数据库，每个30 ms 检测一次。 当前已经上锁了，不会有新增的读请求。
			time.Sleep(time.Millisecond * 30)
		}

		c := pool.Get()
		defer func() {
			err := c.Close()
			if err != nil {
				log.Println("redis pool 关闭失败")
			}
			RWChannel[tempid].isWriting = false
			<-RWChannel[tempid].Write
		}()

		res, err := redis.String(c.Do("get", room))
		if err != nil {
			log.Println(err)
		}

		if res == "" { // NOT FOUND , MYSQL
			msql := "select name, department, status from userInfo where roomId = ? and meetingStartTime = ? and deleted = 0 limit 1"
			rows, err := db.Query(msql, data.RoomId, data.StartTime)
			if err != nil {
				log.Println(err)
			}
			name := ""
			department := ""
			for rows.Next() {
				var status string
				rows.Scan(&name, &department, &status)
				if status == "notSign" || status == "Sign" {
					return errors.New("目标时间的该会议室已经被占用,预约失败")
				}
			}

		} else if res[0] != '0' {
			return errors.New("目标时间的该会议室已经被占用,预约失败")
		}

		// 先向mysql插入数据 ，然后插入redis
		msql := "insert into userInfo(name, phone, email, department, topic, roomId, meetingStartTime, remark, status, deleted, bookTime) VALUES( ?, ?, ?, ?, ?, ?, ?, ? , 'notSign', 0 , now() )"

		_, err = db.Exec(msql, data.UserName, data.UserPhone, data.UserEmail, data.UserDepartment, data.MeetingTopic, data.RoomId, data.StartTime, data.UserRemark)
		if err != nil {
			return errors.New("预约会议室插入mysql失败")
		}

		// 使用lua脚本原子性的插入并且设置过期时间
		dirtyKey := "dirty" + room
		dirtyVal := "0," + value
		// 插入dirty脏数据
		script := "redis.call('SET', KEYS[1], ARGV[1]) ; redis.call('expireat', KEYS[1], KEYS[2]); redis.call('set', ARGV[2], ARGV[3])"
		value = "1"+ value
		formatTime, err := time.Parse("20060102150405", data.StartTime)
		endTime := formatTime.Unix() - 8*60*60 + 15*60
		c.Do("EVAL", script, "2", room, endTime, value, dirtyKey, dirtyVal)

	default:
		return errors.New("服务器繁忙请重试 🐶")
	}
	log.Println("预约会议室成功， 预约人：", data.UserName, "部门： ", data.UserDepartment, "会议室Id+使用时间:", data.RoomId, " + ",data.StartTime)
	return nil
}

// 取消会议室预约
func CancelRoom(data *Data) error {
	room := data.RoomId + data.StartTime

	// 校验会议室是否处于未签入状态
	NowStatus, ok := QueryRoomStatus(data.RoomId, data.StartTime)
	if ok == false {
		return errors.New("查询状态失败")
	}
	if NowStatus != NOTSIGN {
		log.Println("该会议室这个时间段并没有被预约")
		return errors.New("该时间段的会议室并没有被预约")
	}

	// 校验会议室是否是这个人预约的
	if judgePermissions(data.UserName, data.UserDepartment, room) == false {
		log.Println("该会议室不是此人预约")
		return errors.New("无权限，不是您预约的")
	}

	tempid := getMark(room)

	select {
	case RWChannel[tempid].Write <- struct {} {} :
		mu.Lock()
		RWChannel[tempid].isWriting = true
		mu.Unlock()
		for {
			if RWChannel[tempid].isReading == 0 {
				break
			}
			time.Sleep(30 * time.Millisecond)
		}

		c := pool.Get()
		defer func () {
			err := c.Close()
			if err != nil {
				log.Println("close redis pool error")
			}
			mu.Lock()
			RWChannel[tempid].isWriting = false
			<- RWChannel[tempid].Write
			mu.Unlock()
		} ()

		msql := "updata userInfo set deleted = 1 where deleted = 0 and roomId = ? and meetingStartTime = ? and name = ? and department = ?"
		_, err := db.Exec(msql, data.RoomId, data.StartTime, data.UserName, data.UserDepartment)
		if err != nil {
			log.Println("change mysql失败")
		}

		dirtyKey := "dirty" + room
		script := "redis.call('set', KEYS[1], 0); redis.call('del', ARGV[1])"
		_, err = c.Do("EVAL", script, 1, room, dirtyKey)
		if err != nil {
			log.Println(err)
		}

		log.Println("取消预约成功, 取消人: ", data.UserName, " 会议室ID：", data.RoomId, " 使用时间: ", data.StartTime)
		return nil
	default:
		return errors.New("服务器繁忙请重试 🐶")
	}
}

// 签入会议室
func SignRoom(data *Data) error {
	room := data.RoomId + data.StartTime
	value := data.UserDepartment + data.UserName

	// 校验会议室的状态
	NowStatus, ok := QueryRoomStatus(data.RoomId, data.StartTime)
	if ok == false {
		return errors.New("查询会议室状态失败，请重试")
	}
	if NowStatus != NOTSIGN {
		log.Println("该会议室没有被预约")
		return errors.New("该会议室没有被预约")
	}

	// 校验用户权限
	if judgePermissions(data.UserName, data.UserDepartment, room) == false {
		log.Println("您没有权限，请预约当事人操作")
		return errors.New("您没有权限，请预约当事人操作")
	}

	tempid := getMark(room)

	select {
	case RWChannel[tempid].Write <- struct {} {} :
		mu.Lock()
		RWChannel[tempid].isWriting = true
		mu.Unlock()
		for {
			if RWChannel[tempid].isReading == 0 {
				break
			}
			time.Sleep(30 * time.Millisecond)
		}

		c := pool.Get()
		defer func () {
			err := c.Close()
			if err != nil {
				log.Println(err)
			}
			mu.Lock()
			<- RWChannel[tempid].Write
			RWChannel[tempid].isWriting = false
			mu.Unlock()
		} ()

		// update mysql
		msql := "update userInfo SET status = 'Sign' where roomId = ? and meetingStartTime =?  and name = ?  and department = ? and deleted = 0"
		_, err := db.Exec(msql, data.RoomId, data.StartTime, data.UserName, data.UserDepartment)
		if err != nil {
			log.Println("修改mysql状态失败")
		}

		// update redis
		// 使用lua脚本原子性的插入并且设置过期时间
		dirtyKey := "dirty" + room
		dirtyVal := "0," + value
		script := "redis.call('SET', KEYS[1], ARGV[1]) ; redis.call('expireat', KEYS[1], KEYS[2]); " +
			" redis.call('set', ARGV[2], ARGV[3]) "

		value = "2" + value
		formatTime, _ := time.Parse("20060102150405", data.StartTime)
		endTime := formatTime.Unix() - 8*60*60 + 60*60
		_, err = c.Do("EVAL", script, "2", room, endTime, value, dirtyKey, dirtyVal)
		if err != nil {
			log.Println(err)
		}
		log.Println("用户：" + data.UserName + " 已签入会议室Id: " + data.RoomId)
		return nil

	default:
		return errors.New("服务器繁忙请重试 🐶")
	}
}

// 提前结束会议室的使用
func EndUseRoom(data *Data) error {
	// 校验会议室的状态
	NowStatus, ok := QueryRoomStatus(data.RoomId, data.StartTime)
	if ok == false {
		return errors.New("查询会议室状态失败，请重试")
	}
	if NowStatus != NOTSIGN {
		log.Println("该会议室没有被预约")
		return errors.New("该会议室没有被预约")
	}

	// 校验权限
	room := data.RoomId + data.StartTime
	//value := data.UserDepartment + data.UserName
	if judgePermissions(data.UserName, data.UserDepartment, room) == false {
		log.Println("您没有此权限")
		return errors.New("你没有权限")
	}

	tempid := getMark(room)

	select {
	case RWChannel[tempid].Write <- struct {} {} :
		mu.Lock()
		RWChannel[tempid].isWriting = true
		mu.Unlock()
		for {
			if RWChannel[tempid].isReading == 0 {
				break
			}
			time.Sleep(30 * time.Millisecond)
		}

		c := pool.Get()
		defer func () {
			err := c.Close()
			if err != nil {
				log.Println(err)
			}
			mu.Lock()
			RWChannel[tempid].isWriting = false
			<- RWChannel[tempid].Write
			mu.Unlock()
		} ()

		// 修改mysql数据
		msql := "updata userInfo set deleted = 1 where roomId = ? and meetingStartTime = ? and name = ? and department = ? and deleted = 0"
		_, err := db.Exec(msql, data.RoomId, data.StartTime, data.UserName, data.UserDepartment)
		if err != nil {
			return errors.New("删除mysql记录失败")
		}

		//修改redis数据
		dirtyKey := "dirty" + room
		script := "redis.call('set', KEYS[1], 0);" +
			"redis.call('del', ARGV[1]);"
		_, err = c.Do("EVAL", script, 1, room, dirtyKey)
		if err != nil {
			log.Println(err)
		}
		log.Println("提前结束会议室的使用, roomid = ", data.RoomId, " starttime = ", data.StartTime, " 用户 = ", data.UserName)
		return nil

	default:
		return errors.New("服务器繁忙请重试 🐶")
	}

}

// 将最新状态更新到Mysql中
func UpdateData(data *Data) bool {
	NowStatus, ok := QueryRoomStatus(data.RoomId, data.StartTime)
	if ok == false {
		return false
	}
	if NowStatus == FREE {
		db.Exec("delete from userInfo where roomId = ? and meetingStartTime = ?", data.RoomId, data.StartTime)
	} else if NowStatus == NOTSIGN {
		db.Exec("update userInfo SET status = 'notSign' where roomid = ? and meetingStartTime = ?", data.RoomId, data.StartTime)
	} else if NowStatus == USING {
		db.Exec("update userInfo SET status = 'Sign' where roomid = ? and meetingStartTime = ?", data.RoomId, data.StartTime)
	}
	return true
}


/*
删掉脏链表，使用redis替代掉。
使用一个线程去SCAN redis脏链表的数据，然后更新到mysql中。
set dirty+roomid+stattime 0,unixTime
for each dirty* if nowTime >= unixTime : update mysql data


一致性问题：比如先写mysql之后崩溃，没有写redis。 解决：写完mysql就给用户返回成功，写入redis的可以异步去做？ 好像不能，写数据还是要用互斥量的。
set roomidstarttime value
 */