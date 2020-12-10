package model

import (
	"database/sql"
	"errors"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gomodule/redigo/redis"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	FREE    = 0
	NOTSIGN = 1
	USING   = 2
)

var STATUS = []string{"FREE", "NOTSIGN", "USING"}

var abort chan Output
var success chan Output

var rw sync.RWMutex
var mu sync.Mutex

var ID int // 唯一ID

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

type Output struct {
	Email string
	Department string
	Name string
	ErrReason string
}

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
	db, _ = sql.Open("mysql", "root:Xtm_0124@tcp(cdb-027nnpt2.cd.tencentcdb.com:10106)/GinMeetRoom")

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	c := pool.Get()
	defer c.Close()
	var err error
	ID , err = redis.Int(c.Do("get", "ID"))
	if err != nil {
		log.Println("获取ID失败.", err)
	}

	Mark = make(map[string]int)
	abort = make(chan Output, 100)
	success = make(chan Output, 100)
	for i := 0; i < 1000; i++ { // 设置默认值
		RWChannel = append(RWChannel, *NewChannel())
	}
}

func GetAllRoomInfo (ret map[string]bool) error {
	msql := "select RoomId from roomInfo where deleted = 0"
	rows, err := db.Query(msql)
	if err != nil {
		log.Println("查询所有会议室失败， err = ", err)
		return errors.New("查询所有会议室信息失败， ERR = " + err.Error())
	}
	for rows.Next() {
		var roomid string
		err := rows.Scan(&roomid)
		if err != nil {
			log.Println("mysql scan failed. err = ", err.Error())
		}
		ret[roomid] = true
	}
	return nil
}

// 输入roomid+starttime
func getMark(str string) int {
	rw.RLock()
	res, ok := Mark[str]
	rw.RUnlock()

	if !ok {
		res = setMark(str)
	}
	return res
}

// input roomid+starttime
func setMark(str string) int {
	rw.Lock()

	res, ok := Mark[str]
	if ok {
		rw.Unlock()
		return res
	}

	c := pool.Get()
	defer c.Close()

	redisKey := "ID" + str
	val, err := redis.String(c.Do("get", redisKey))
	if err != nil {
		log.Println("get Mark key failed。",err)
	}
	if val == "" {
		_, err := c.Do("set", redisKey, ID)
		if err != nil {
			log.Println("设置ID失败.", err)
		}
		Mark[str] = ID
		val = strconv.Itoa(ID)

		log.Println("新设置一个ID，会议室信息：", str, " 分配的ID = ", ID)

		ID++
		_, err = c.Do("set", "ID", ID)
		if err != nil {
			log.Println("更新最大ID失败。", err)
		}
	} else {
		Mark[str], _ = strconv.Atoi(val)
	}

	if ID > len(RWChannel) { // 当分配的ID超过数组大小时要设置好默认值,按照2倍提前扩容
		templen := len(RWChannel)
		for i := templen; i <= ID*2; i++ {
			RWChannel = append(RWChannel, *NewChannel())
		}
	}
	defer rw.Unlock()
	return Mark[str]
}

// 验证当前时间是否已经超过会议室结束时间
func judgeTimeCorrect(startTime string) bool {
	formatTime, _ := time.Parse("20060102150405", startTime)
	lastTime      := formatTime.Unix() - 8*60*60 + 60 * 60
	nowUnix       := time.Now().Unix()
	if nowUnix >= lastTime {
		log.Println("订阅时间错误,当前预定时间为：", startTime, " nowUnix = ", nowUnix, " lastTime = ", lastTime)
		return false
	}
	return true
}

func judgePermissionNolock (username, userdepartment, key string) bool {
	c := pool.Get()
	defer c.Close()

	value, err := redis.String(c.Do("get", key))
	if err != nil {
		log.Println("redis get failed")
	}
	user := userdepartment + username
	log.Println("judgePermission: value = ", value[1:], " user = ", user)
	return value[1:] == user
}

// 验证用户数据信息和会议室预约人是否一致
// input ：key是roomid+starttime
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
			log.Println("value = ", value[1:], " user = ", user)
			return value[1:] == user
		}
	}
	return false
}

// 查询所有空闲会议室
func QueryAllFreeRoom() ([]string, error) {
	// 向所有会议室全部isreading+1， 拿到所有权限保证不会被写。
	success := 0
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
				success++
				mu.Unlock()
			}
		}
	}

	if success != ID {
		log.Println("没能获取所有的权限")
		return nil, errors.New("查询失败")
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
	// lua脚本使用SCAN异步获取所有空闲的会议室，然后返回。
	script := "local ans, has, cursor = {}, {}, \"0\"\nrepeat\n    local t = redis.call(\"SCAN\", cursor)\n    local list = t[2]\n    for i = 1, #list do\n        local s = list[i]\n        if has[s] == nil then\n            has[s] = 1\n            local value = redis.call(\"get\", s)\n            if value == \"0\" then\n                ans[#ans + 1] = s\n            end\n        end\n    end\n    cursor = t[1]\nuntil cursor == \"0\"\nreturn ans --or return ans"
	arr, err := redis.Values(c.Do("eval", script, 0))
	if err != nil {
		return ret, errors.New(err.Error())
	}
	for _, v := range arr {
		mu.Lock()
		ret = append(ret, string(v.([]byte))) // append 不是并发安全的
		mu.Unlock()
	}
	return ret, nil
}

// value is value in redis key-value
func cacheData(tempid int, roomid, starttime, value string) { // 使用协程去写入redis，在读的情况下不能够写入。
	select {
	case RWChannel[tempid].Write <- struct{}{}:
		log.Println("cacheData got write power!!!")
		mu.Lock()
		RWChannel[tempid].isWriting = true
		mu.Unlock()

		for {
			if RWChannel[tempid].isReading == 0 {
				break
			}
			time.Sleep(30 * time.Millisecond)
			log.Println("i am waiting for isReading = 0")
		}

		c := pool.Get()
		defer func(temp int) {
			err := c.Close()
			if err != nil {
				log.Println("redis close error !")
			}
			mu.Lock()
			RWChannel[temp].isWriting = false
			mu.Unlock()
			<-RWChannel[temp].Write
		}(tempid)

		if _, err := redis.Int(c.Do("set", roomid+starttime, value)); err != nil {
			log.Println(err)
		}
	default:
		return
	}
}

// 查询某时间段会议室状态 加锁可以单独调用
func QueryRoomStatus(RoomId, StartTime string) (int, bool) {
	room  := RoomId + StartTime
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
			msql := "select name, department, status from userInfo where roomId = ? and meetingStartTime = ? and deleted = 0 limit 1"
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

// 查询状态，无锁只能在有锁环境下调用
func QueryRoomStatusNolock(RoomId, StartTime string) (int, bool) {
	room  := RoomId + StartTime

	c := pool.Get()
	defer func() {
		err := c.Close()
		if err != nil {
			log.Println("close redis pool error. ", err)
		}
	}()

	res, err := redis.String(c.Do("get", room)) // 首先从redis缓存中查询
	if err != nil {
		log.Println("查询redis失败，重试中")
		res, err = redis.String(c.Do("get", room))
		if err != nil {
			log.Println("查询redis再次失败")
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
	msql := "select name, department, status from userInfo where roomId = ? and meetingStartTime = ? and deleted = 0 limit 1"
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
	if _, err := redis.Int(c.Do("set", room, val)); err != nil {
		log.Println(err)
	}
	return retStatus, true // 强行保证defer函数一定能够执行，因为上述都没能够返回会导致 defer 资源泄漏
}

/*
用于预定的准备，如果一切正常则返回tempid(会议室号+使用时间的唯一ID）和nil。后续可以开一个协程去写入DB，然后异步返回给用户。
如果出现错误，就返回错误信息。
 */
func BookPrepare(data *Data) (int, error) {
	if !judgeTimeCorrect(data.StartTime) {
		return 0, errors.New("BookRoom订阅时间错误")
	}
	room := data.RoomId + data.StartTime
	tempid := getMark(room)

	select {
	case RWChannel[tempid].Write <- struct {} {} :
		mu.Lock()
		RWChannel[tempid].isWriting = true
		mu.Unlock()

		for {
			if RWChannel[tempid].isReading == 0 { // 保证没有在reading的情况
				break
			}
			time.Sleep(5 * time.Millisecond)
		}

		NowStatus, ok := QueryRoomStatusNolock(data.RoomId, data.StartTime)
		if !ok {
			mu.Lock()
			RWChannel[tempid].isWriting = false
			mu.Unlock()
			<- RWChannel[tempid].Write
			return 0,errors.New("查询会议室状态失败")
		}
		log.Printf("预约者： %s， 会议室：%s 使用时间：%s，当前状态为：%s", data.UserName, data.RoomId, data.StartTime, STATUS[NowStatus])

		if NowStatus != FREE {
			mu.Lock()
			RWChannel[tempid].isWriting = false
			mu.Unlock()
			<- RWChannel[tempid].Write
			log.Println("目标时间的该会议室已经被占用,预约失败.")
			return 0, errors.New("目标时间的该会议室已经被占用,预约失败")
		}
		return tempid, nil
	default:
		mu.Lock()
		RWChannel[tempid].isWriting = false
		mu.Unlock()
		<- RWChannel[tempid].Write
		return 0, errors.New("服务器繁忙请重试 🐶")
	}
}

// 真正写入DB数据 如果出现错误则写入abort，否则写入success
func BookRoom (data *Data, tempid int)  {
	log.Println("BookRoom开始写入DB数据")
	c := pool.Get()
	defer func () {
		err := c.Close()
		if err != nil {
			log.Println("close redis link failed. err = ", err)
		}
		mu.Lock()
		RWChannel[tempid].isWriting = false
		mu.Unlock()
		<- RWChannel[tempid].Write
	} ()

	tempOutput := Output{
		Name: data.UserName,
		Email: data.UserEmail,
		Department: data.UserDepartment,
	}

	// 先向mysql插入数据 ，然后插入redis
	// start transaction 使用事务插入并且获取刚插入的uid，用于dirtyVal唯一性的更新数据
	tx, err := db.Begin()
	if err != nil {
		tempOutput.ErrReason += "开启事务失败. err = " + err.Error() + "\n"
		log.Println("开启事务失败. err = ", err)
	}
	insertTx, err := tx.Prepare("insert into userInfo(name, phone, email, department, topic, roomId, meetingStartTime, remark, status, deleted, bookTime) VALUES( ?, ?, ?, ?, ?, ?, ?, ? , 'notSign', 0 , now() )")
	if err != nil {
		tempOutput.ErrReason += "prepare failed. err = " + err.Error() + "\n"
		log.Println("准备失败 prepare failed。 err = ", err)
	}
	_, err = insertTx.Exec(data.UserName, data.UserPhone, data.UserEmail, data.UserDepartment, data.MeetingTopic, data.RoomId, data.StartTime, data.UserRemark)
	if err != nil {
		tempOutput.ErrReason += "insert mysql failed. err = " + err.Error() + "\n"
		log.Println("预约会议室插入mysql失败。 err = ", err)
	}
	queryTx, err := tx.Prepare("select uid from userInfo where roomId = ? and meetingStartTime = ? and deleted = 0 limit 1")
	if err != nil {
		tempOutput.ErrReason += "prepare mysql failed. err = " + err.Error() + "\n"
		log.Println("准备语句失败。 err = ", err)
	}
	rows, err := queryTx.Query(data.RoomId, data.StartTime)
	if err != nil {
		tempOutput.ErrReason += "query uid failed. err = " + err.Error() + "\n"
		log.Println("查询uid失败。 err = ",err)
	}
	uid := 0
	for rows.Next() {
		err = rows.Scan(&uid)
		if err != nil {
			log.Println(err)
		}
	}
	err = tx.Commit()
	if err != nil {
		tempOutput.ErrReason += "transaction commit failed. err = " + err.Error() + "\n"
		log.Println("事务结束失败。 err = ", err)
	}
	// mysql 正确执行之后返回预约成功 TODO xutianmeng

	// 使用lua脚本原子性的插入并且设置过期时间
	// 插入dirty脏数据
	room := data.RoomId + data.StartTime
	script := "redis.call('SET', KEYS[1], KEYS[2]); " +
		"redis.call('SET', KEYS[3], KEYS[4]); "
	value := "1" + data.UserDepartment + data.UserName
	formatTime, err := time.Parse("20060102150405", data.StartTime)
	if err != nil {
		tempOutput.ErrReason += "获取startTime的unix格式失败. err = " + err.Error() + "\n"
		log.Println("获取startTime的unix格式失败， ERR = ",err)
	}
	endTime := formatTime.Unix() - 8*60*60 + 15*60
	if time.Now().Unix() >= endTime { 	// 如果预约的是超时自动释放的会议室，那么签入超时的时间需要重新设置。

	}
	dirtyKey := "dirty" + room
	dirtyVal := strconv.Itoa(uid) + "," + strconv.FormatInt(endTime, 10)
	_,err = c.Do("EVAL", script, "4", room, value, dirtyKey, dirtyVal)
	if err != nil {
		tempOutput.ErrReason += "EVAL script error. err = " + err.Error() + "\n"
		log.Println("EVAL执行脚本出错", err)
	}
	if tempOutput.ErrReason == "" {
		success <- tempOutput
	} else {
		abort <- tempOutput
	}
	log.Println("预约会议室成功， 预约人：", data.UserName, "部门： ", data.UserDepartment, "会议室Id: ", data.RoomId, " 使用时间: ",data.StartTime)
}

func CancelPrepare (data *Data) (int,error) {
	if !judgeTimeCorrect(data.StartTime) {
		return 0, errors.New("CancelRoom预定时间错误")
	}
	room := data.RoomId + data.StartTime
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
			time.Sleep(5 * time.Millisecond)
		}

		NowStatus, ok := QueryRoomStatusNolock(data.RoomId, data.StartTime)
		if !ok {
			mu.Lock()
			RWChannel[tempid].isWriting = false
			mu.Unlock()
			<- RWChannel[tempid].Write
			return 0,errors.New("查询会议室状态失败")
		}

		if NowStatus != NOTSIGN {
			mu.Lock()
			RWChannel[tempid].isWriting = false
			mu.Unlock()
			<- RWChannel[tempid].Write
			if NowStatus == FREE {
				return 0, errors.New("目标时间的会议室处于空闲状态！")
			}
			if NowStatus == USING {
				return 0, errors.New("目标时间的会议室正在使用中!")
			}
		}
		if !judgePermissionNolock(data.UserName, data.UserDepartment, room) {
			return 0, errors.New("无权限！")
		}

		return tempid, nil

	default:
		return 0, errors.New("服务器繁忙请重试 🐶")
	}
}

func CancelRoom (data *Data, tempid int) {
	log.Println("CancelRoom开始写入DB数据")
	c := pool.Get()
	defer func () {
		err := c.Close()
		if err != nil {
			log.Println(err)
		}
		mu.Lock()
		RWChannel[tempid].isWriting = false
		mu.Unlock()
		<- RWChannel[tempid].Write
	} ()

	tempOutput := Output{
		Name: data.UserName,
		Email: data.UserEmail,
		Department: data.UserDepartment,
	}
	msql := "update userInfo set deleted = 1 where deleted = 0 and roomId = ? and meetingStartTime = ? and name = ? and department = ?"
	_, err := db.Exec(msql, data.RoomId, data.StartTime, data.UserName, data.UserDepartment)
	if err != nil {
		tempOutput.ErrReason += " update mysql failed. err = " + err.Error() + "\n"
		log.Println("update mysql失败. ", err)
	}

	room := data.RoomId + data.StartTime
	dirtyKey := "dirty" + room
	script := "redis.call('SET', KEYS[1], 0); " +
			  "redis.call('DEL', ARGV[1])"
	_, err = c.Do("EVAL", script, 1, room, dirtyKey)
	if err != nil {
		tempOutput.ErrReason += "execute redis lua script failed. err= " + err.Error() + "\n"
		log.Println(err)
	}
	if tempOutput.ErrReason == "" {
		success <- tempOutput
	} else {
		abort <- tempOutput
	}
	log.Println("取消预约成功, 取消人: ", data.UserName, " 会议室ID：", data.RoomId, " 使用时间: ", data.StartTime)
}

func SignPrepare (data *Data) (int, error) {
	if !judgeTimeCorrect(data.StartTime) {
		return 0, errors.New("SignRoom选择时间错误")
	}
	room := data.RoomId + data.StartTime
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
			time.Sleep(5 * time.Millisecond)
		}

		NowStatus, ok := QueryRoomStatusNolock(data.RoomId, data.StartTime)
		if !ok {
			mu.Lock()
			RWChannel[tempid].isWriting = false
			mu.Unlock()
			<- RWChannel[tempid].Write
			return 0,errors.New("查询会议室状态失败")
		}

		if NowStatus != NOTSIGN {
			mu.Lock()
			RWChannel[tempid].isWriting = false
			mu.Unlock()
			<- RWChannel[tempid].Write
			if NowStatus == FREE {
				return 0, errors.New("目标时间的会议室处于空闲状态！")
			}
			if NowStatus == USING {
				return 0, errors.New("目标时间的会议室正在使用中! ")
			}
		}
		if !judgePermissionNolock(data.UserName, data.UserDepartment, room) {
			return 0, errors.New("无权限！")
		}

		return tempid, nil
	default:
		return 0, errors.New("服务器繁忙请重试 🐶")
	}
}

func SignRoom (data *Data, tempid int) {
	log.Println("SignRoom开始写入DB数据")
	c := pool.Get()
	defer func () {
		err := c.Close()
		if err != nil {
			log.Println("close redis failed. err = ", err)
		}
		mu.Lock()
		RWChannel[tempid].isWriting = false
		mu.Unlock()
		<- RWChannel[tempid].Write
	} ()

	tempOutput := Output{
		Name: data.UserName,
		Email: data.UserEmail,
		Department: data.UserDepartment,
	}

	// update mysql
	// start transaction 使用事务先更新mysql数据并获取uid，用于dirtyVAL
	tx, err := db.Begin()
	if err != nil {
		tempOutput.ErrReason += "开启事务失败. err = " + err.Error() + "\n"
		log.Println("开启事务失败。 err = ", err)
	}
	updateTx, err := tx.Prepare("update userInfo SET status = 'Sign' where roomId = ? and meetingStartTime = ?  and name = ?  and department = ? and deleted = 0")
	if err != nil {
		tempOutput.ErrReason += "mysql prepare FAILED. err = " + err.Error() + "\n"
		log.Println("mysql prepare FAILED. err = ", err)
	}
	_, err = updateTx.Exec(data.RoomId, data.StartTime, data.UserName, data.UserDepartment)
	if err != nil {
		tempOutput.ErrReason += "修改mysql状态失败. err = " + err.Error() + "\n"
		log.Println("修改mysql状态失败， err = ", err)
	}
	selectTx, err := tx.Prepare("select uid from userInfo where roomId = ? and meetingStartTime = ? and name = ? and department = ? and deleted = 0")
	if err != nil {
		tempOutput.ErrReason += "mysql prepare FAILED. err = " + err.Error() + "\n"
		log.Println("mysql prepare FAILED, err = ", err)
	}
	rows, err := selectTx.Query(data.RoomId, data.StartTime, data.UserName, data.UserDepartment)
	if err != nil {
		tempOutput.ErrReason += "查询mysql uid 失败. err = " + err.Error() + "\n"
		log.Println("查询mysql uid 失败， err = ", err)
	}
	uid := 0
	for rows.Next() {
		rows.Scan(&uid)
	}
	err = tx.Commit()
	if err != nil {
		tempOutput.ErrReason += "关闭事务失败. err = " + err.Error() + "\n"
		log.Println("关闭事务失败, err = ", err)
	}

	// update redis 使用lua脚本原子性的插入并且设置过期时间
	script := "redis.call('SET', KEYS[1], ARGV[1]) ; " +
		"redis.call('SET', ARGV[2], ARGV[3]) "

	room := data.RoomId + data.StartTime
	value := "2" + data.UserDepartment + data.UserName
	formatTime, _ := time.Parse("20060102150405", data.StartTime)
	endTime := formatTime.Unix() - 8*60*60 + 60*60
	dirtyKey := "dirty" + room
	dirtyVal := strconv.Itoa(uid) + "," + strconv.FormatInt(endTime, 10)
	_, err = c.Do("EVAL", script, "1", room, value, dirtyKey, dirtyVal)
	if err != nil {
		tempOutput.ErrReason += "execute redis lua script failed. err = " + err.Error() + "\n"
		log.Println(err)
	}

	if tempOutput.ErrReason == "" {
		success <- tempOutput
	} else {
		abort <- tempOutput
	}
	log.Println("用户：" + data.UserName + " 已签入会议室Id: " + data.RoomId)
}

func StopPrepare (data *Data) (int, error) {
	if !judgeTimeCorrect(data.StartTime) {
		return 0, errors.New("EndUseRoom选择时间错误")
	}

	room := data.RoomId + data.StartTime
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
			time.Sleep(5 * time.Millisecond)
		}

		NowStatus, ok := QueryRoomStatusNolock(data.RoomId, data.StartTime)
		if !ok {
			mu.Lock()
			RWChannel[tempid].isWriting = false
			mu.Unlock()
			<- RWChannel[tempid].Write
			return 0,errors.New("查询会议室状态失败")
		}

		if NowStatus != NOTSIGN {
			mu.Lock()
			RWChannel[tempid].isWriting = false
			mu.Unlock()
			<- RWChannel[tempid].Write
			if NowStatus == FREE {
				return 0, errors.New("目标时间的会议室处于空闲状态！")
			}
			if NowStatus == NOTSIGN {
				return 0, errors.New("目标时间的会议室处于未签入状态! ")
			}
		}
		if !judgePermissionNolock(data.UserName, data.UserDepartment, room) {
			return 0, errors.New("无权限！")
		}
		return tempid, nil
	default:
		return 0, errors.New("服务器繁忙请重试 🐶")
	}
}

func StopRoom (data *Data, tempid int) {
	log.Println("StopRoom开始写入DB数据")
	c := pool.Get()
	defer func() {
		err := c.Close()
		if err != nil {
			log.Println(err)
		}
		mu.Lock()
		RWChannel[tempid].isWriting = false
		mu.Unlock()
		<-RWChannel[tempid].Write
	}()

	tempOutput := Output{
		Name:       data.UserName,
		Email:      data.UserEmail,
		Department: data.UserDepartment,
		ErrReason:  "",
	}

	// 修改mysql数据
	msql := "update userInfo set deleted = 1 where roomId = ? and meetingStartTime = ? and name = ? and department = ? and deleted = 0"
	_, err := db.Exec(msql, data.RoomId, data.StartTime, data.UserName, data.UserDepartment)
	if err != nil {
		tempOutput.ErrReason += "删除mysql记录失败, err = " + err.Error() + "\n"
		log.Println("删除mysql记录失败, err = ", err)
	}

	room := data.RoomId + data.StartTime
	//修改redis数据
	dirtyKey := "dirty" + room
	script := "redis.call('SET', KEYS[1], 0);" +
		"redis.call('DEL', ARGV[1]);"
	_, err = c.Do("EVAL", script, 1, room, dirtyKey)
	if err != nil {
		tempOutput.ErrReason += "execute redis lua script, err = " + err.Error() + "\n"
		log.Println(err)
	}
	if tempOutput.ErrReason == "" {
		success <- tempOutput
	} else {
		abort <- tempOutput
	}
	log.Println("提前结束会议室的使用, roomid = ", data.RoomId, " StartTime = ", data.StartTime, " User = ", data.UserName)
}

// 定时执行，去扫描redis中的dirty数据
func UpdateDirtyData () {
	script := "local ans, has, cursor = {}, {}, \"0\"\nrepeat\n    local t = redis.call('SCAN', cursor, \"MATCH\", \"dirty*\")\n    cursor = t[1]\n    local list = t[2]\n    for i = 1, #list do\n        local key = list[i]\n        if has[key] == nil then\n            has[key] = 1\n            local value = redis.call('GET', key)\n            ans[#ans+1] = key .. \",\" .. value\n        end\n    end\nuntil cursor == \"0\"\nreturn ans"
	c := pool.Get()
	defer c.Close()

	arr, err := redis.Values(c.Do("EVAL", script, "0"))
	if err != nil {
		log.Println("执行更新脏数据的命令出错", err)
	}
	for _, v := range arr {
		str := string(v.([]byte))
		tempArr := strings.Split(str, ",") // [0] = dirtyRoomIdStartTime [1] = unixTime

		key 	  := tempArr[0]
		dirtyKey  := tempArr[0]
		key        = key[5:]
		roomId 	  := key[0:3]
		startTime := key[3:]

		tempArr = strings.Split(tempArr[1], ",")
		uid, err := strconv.Atoi(tempArr[0])
		if err != nil {
			log.Println("转换uid失败, err = ", err)
		}
		deadline, err := strconv.ParseInt(tempArr[1], 10, 64)
		if err != nil {
			log.Println("转换int失败。", err)
		}

		nowTime := time.Now().Unix()

		if nowTime >= deadline { // 过期，需要更新数据了
			tempid := getMark(key)
			for {
				select {
				case RWChannel[tempid].Write <- struct {} {} :
					mu.Lock()
					RWChannel[tempid].isWriting = true
					mu.Unlock()
					for {
						if RWChannel[tempid].isReading == 0 {
							break
						}
						time.Sleep(5 * time.Millisecond)
					}

					c := pool.Get()
					defer func () {
						err := c.Close()
						if err != nil {
							log.Println("close pool resource failed.", err)
						}
						mu.Lock()
						<- RWChannel[tempid].Write
						RWChannel[tempid].isWriting = false
						mu.Unlock()
					} ()

					// update data 使用uid 作为唯一标识，即使程序崩溃导致mysql重复设置DELETED = 1也不会影响数据正确性
					// 超时转为空闲 -> mysql 标记 deleted = 1, redis set room 0, del dirtyroom
					msql := "update userInfo set deleted = 1 where roomId = ? meetingStartTime = ? and uid = ? and deleted = 0"
					_, err := db.Exec(msql, roomId, startTime, uid)
					if err != nil {
						log.Println("删除mysql记录失败")
					}
					tempscript := "redis.call('SET', KEYS[1], \"0\")\nredis.call('DEL', KEYS[2])"
					_,err = c.Do("EVAL", tempscript, "2", key, dirtyKey)
					if err != nil {
						log.Println("执行更新记录的lua脚本失败。", err)
					}
				default:
					time.Sleep(5 * time.Millisecond)
					continue
				}
			}
		}
	}
}

// 接收异步写成传入的信息
func NotifyOutput() {
	for {
		select {
		case successMsg := <- success:
			log.Println(successMsg) // 模拟发送邮件
		case errMsg := <- abort :
			log.Println(errMsg)
		}
	}
}


/*
删掉脏链表，使用redis替代掉。
使用一个线程去SCAN redis脏链表的数据，然后更新到mysql中。
set dirty+roomid+stattime 0,unixTime
如果当前时间超过了unixTime，就说明状态变成了FREE
for each dirty* if nowTime >= unixTime : update mysql data

更新redis中脏数据的时候，首先通过SCAN所有数据，然后当对一些数据更新到MYSQL之后程序崩溃，没有执行lua脚本也就没有删掉redis脏数据。
那么会有以下两个问题：
1. 会议室超出使用时间而过期，那么不会有任何问题。因为对于超时的会议室进行数据修改，都会被拒绝，所以哪怕进行多次更新mysql数据也不会有问题。
2. 会议室超出签入时间而过期，会有问题。数据在redis中被过期，有人预约这间会议室的时候，会跳过redis直接查询mysql，mysql的数据被更新过是正确的，
然后正常的进行了写入预约。此时redis的脏数据开始更新到mysql上去，会把新预约的信息标记为DELETED = 1。

一致性问题：比如先写mysql之后崩溃，没有写redis。 解决：写完mysql就给用户返回成功，通过一个管道传送过去，
		前端收到就立刻给用户反馈成功，服务端继续处理redis缓存

dirtyVal = uid,unixTime

不对redis的数据设置过期时间了，在更新的时候再去重制。
 */