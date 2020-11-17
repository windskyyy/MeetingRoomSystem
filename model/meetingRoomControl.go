package model

import (
	"database/sql"
	"encoding/json"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gomodule/redigo/redis"
	"log"
	"sync"
	"time"
)

const (
	FREE    = 0
 	NOTSIGN = 1
 	USING   = 2
)

var rw sync.RWMutex

var pool *redis.Pool  //创建redis连接池
var db *sql.DB

var NotSignHead *NotSignLink
var SignHead *SignLink

var NotSignMap map[string]*NotSignLink
var SignMap map[string]*SignLink

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
	return &Data {
		RoomId: "",
		UserName: "",
		UserPhone: "",
		UserEmail: "",
		StartTime: "",
		UserRemark: "",
		MeetingTopic: "",
		UserDepartment: "",
	}
}

func init(){
	// redis pool
	pool = &redis.Pool{     //实例化一个连接池
		MaxIdle:16,    //最初的连接数量
		MaxActive:0,    //连接池最大连接数量,不确定可以用0（0表示自动定义），按需分配
		IdleTimeout:300,    //连接关闭时间 300秒 （300秒不使用自动关闭）
		Dial: func() (redis.Conn ,error){     //要连接的redis数据库
			return redis.Dial("tcp","127.0.0.1:6379")
		},
	}

	// mysql
	db, _ = sql.Open("mysql", "root:Xtm_0124@tcp(cdb-027nnpt2.cd.tencentcdb.com:10106)/GinMeetRoom")

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	// LinkHeadNode
	NotSignHead = NewNotSign()
	SignHead = NewSign()

	NotSignMap = make(map[string]*NotSignLink, 0)
	SignMap = make(map[string]*SignLink, 0)
}

// 验证用户数据信息和会议室预约人是否一致
// input ：用户名，用户部门， [notSign | Sign], 要查询的会议室名+使用时间
func judgePermissions(username, userdepartment, status, field string) bool {
	rw.RLock()
	defer rw.RUnlock()

	c := pool.Get()
	defer c.Close()

	key := status + field
	value, _ := redis.String(c.Do("get", key))

	user := userdepartment + username

	return value == user
}

// 查询所有空闲会议室
func QueryAllFreeRoom() []string {
	rw.RLock()
	defer rw.RUnlock()

	c := pool.Get()
	defer c.Close()

	isFree := make(map[string]bool, 0)
	ret := make([]string, 0)

	resKeys, err := redis.Values(c.Do("keys", "allRoom*"))
	if err != nil {
		log.Println("获取allRoom keys失败， ERR = ", err)
	}
	for _, v := range resKeys {
		tempStr := string(v.([]byte))
		str := tempStr[7:]
		isFree[str] = true
	}

	resKeys, err = redis.Values(c.Do("keys", "notSign*"))
	if err != nil {
		log.Println("获取nogSign keys失败， ERR = ", err)
	}
	for _, v := range resKeys {
		tempStr := string(v.([]byte))
		str := tempStr[7:]
		isFree[str] = false
	}

	resKeys, err = redis.Values(c.Do("keys", "Sign*"))
	if err != nil {
		log.Println("获取Sign keys失败， ERR = ", err)
	}
	for _, v := range resKeys {
		tempStr := string(v.([]byte))
		str := tempStr[4:]
		isFree[str] = false
	}

	for str, isFree := range isFree {
		if isFree == true {
			ret = append(ret, str)
		}
	}
	return ret
}

// 查询某时间段会议室状态
func QueryRoomStatus(data *Data) int {
	rw.RLock()
	defer rw.RUnlock()

	c := pool.Get()
	defer c.Close()

	room := data.RoomId + data.StartTime

	key := "notSign" + room
	if flag, _ := redis.Int(c.Do("exists", key)); flag == 1 {
		return NOTSIGN
	}

	key = "Sign" + room
	if flag, _ := redis.Int(c.Do("exists", key)); flag == 1 {
		return USING
	}

	key = "allRoom" + room
	if flag, _ := redis.Int(c.Do("exists", key)); flag == 1 {
		return FREE
	}

	msql := "select status from userInfo where roomId = ? and meetingStartTime = ?"
	rows, err := db.Query(msql, data.RoomId, data.StartTime)
	if err != nil {
		log.Println(err)
	}
	for rows.Next() {
		var status string
		rows.Scan(&status)
		if status == "notSign" {
			return NOTSIGN
		}
		if status == "Sign" {
			return USING
		}
	}
	return FREE
}


// 预约会议室
// bookRoom  返回0表示正常 返回1 表示会议室已被占用
func BookRoom(msg []byte) int {
	var data Data
	_ = json.Unmarshal(msg, &data)

	value := data.UserDepartment + data.UserName
	room := data.RoomId + data.StartTime

	NowStatus := QueryRoomStatus(&data)

	if NowStatus != FREE {
		log.Println("目标时间的该会议室已经被占用,预约失败")
		return 1
	}

	rw.Lock()
	defer rw.Unlock()

	c := pool.Get()
	defer c.Close()

	// 插入redis notSign
	key := "notSign" + room
	_, err := c.Do("set", key, value)
	if err != nil {
		log.Println("预约会议室插入redis失败")
	}

	// 设置过期时间
	formatTime, err := time.Parse("20060102150405",data.StartTime)
	endTime := formatTime.Unix()-8*60*60+15

	key = "notSign" + room
	_, _ = c.Do("expireat", key, int32(endTime))

	// 插入mysql 预约者信息
	msql := "insert into userInfo(name, phone, email, department, topic, roomId, meetingStartTime, remark, status, bookTime) VALUES( ?, ?, ?, ?, ?, ?, ?, ? , 'notSign' , now() )"

	_, err = db.Exec(msql, data.UserName, data.UserPhone, data.UserEmail, data.UserDepartment, data.MeetingTopic, data.RoomId, data.StartTime, data.UserRemark)
	if err != nil {
		log.Println("预约会议室插入mysql失败")
	}

	// 将结点插入NotSign链表
	node := NewNotSign()
	node.insertNode(NotSignHead)
	NotSignMap[room] = node

	log.Println("预约会议室成功， 预约人：", data.UserName, "部门： ", data.UserDepartment, "会议室Id+使用时间:", data.RoomId, data.StartTime)
	return 0
}

// 取消会议室预约
func CancelRoom(msg []byte) int {
	var data Data
	_ = json.Unmarshal(msg, &data)

	room := data.RoomId + data.StartTime

	// 校验会议室是否处于未签入状态
	NowStatus := QueryRoomStatus(&data)
	if NowStatus != NOTSIGN {
		log.Println("该会议室这个时间段并没有被预约")
		return 1
	}

	// 校验会议室是否是这个人预约的
	if judgePermissions(data.UserName, data.UserDepartment, "notSign", room) == false {
		log.Println("该会议室不是此人预约")
		return 2
	}

	rw.Lock()
	defer rw.Unlock()

	c := pool.Get()
	defer c.Close()

	// delete redis notSign
	key := "notSign" + room
	_, err := c.Do("del", key)
	if err != nil {
		log.Println("删除redis notSign失败")
	}

	msql := "delete from userInfo where roomId = ? and meetingStartTime = ? and name = ? and department = ? "

	_, err = db.Exec(msql, data.RoomId, data.StartTime, data.UserName, data.UserDepartment)
	if err != nil {
		log.Println("删除mysql失败")
	}

	// 删除linkNode
	tempNode := NotSignMap[room]
	tempNode.delNode()

	log.Println("取消预约成功, 取消人: ", data.UserName, " 会议室ID：", data.RoomId, " 使用时间: ", data.StartTime)
	return 0
}

// 签入会议室
func SignRoom(msg []byte) int {
	var data Data
	_ = json.Unmarshal(msg, &data)

	room := data.RoomId + data.StartTime
	value := data.UserDepartment + data.UserName

	// 校验会议室的状态
	NowStatus := QueryRoomStatus(&data)
	if NowStatus != NOTSIGN {
		log.Println("该会议室没有被预约")
		return 1
	}

	// 校验用户权限
	if judgePermissions(data.UserName, data.UserDepartment, "notSign", room) == false {
		log.Println("您没有权限，请预约当事人操作")
		return 2
	}

	rw.Lock()
	defer rw.Unlock()

	c := pool.Get()
	defer c.Close()

	// 删除redis notSign
	key := "notSign" + room
	_, err := c.Do("del", key)
	if err != nil {
		log.Println("删除redis notSign失败")
	}

	// 插入redis Sign
	key = "Sign" + room
	_, err = c.Do("set", key, value)
	if err != nil {
		log.Println("插入redis Sign失败")
	}

	// 设置过期时间
	formatTime, err := time.Parse("20060102150405",data.StartTime)
	endTime := formatTime.Unix()-8*60*60+60
	_, err = c.Do("expireat", key,  int32(endTime))
	if err != nil {
		log.Println("设置过期时间失败")
	}

	// 修改DB预约状态为已签入
	msql := "update userInfo SET status = 'Sign' where roomid = ? and meetingStartTime =?  and name = ?  and department = ?"
	_, err = db.Exec(msql, data.RoomId, data.StartTime, data.UserName, data.UserDepartment)
	if err != nil {
		log.Println("修改mysql状态失败")
	}

	// 删除NotSign结点，插入Sign
	tempNotSignNode := NotSignMap[room]
	tempNotSignNode.delNode()

	tempSignNode := NewSign()
	tempSignNode.insertNode(SignHead)
	SignMap[room] = tempSignNode

	log.Println("用户：" + data.UserName + " 已签入会议室Id: " + data.RoomId)
	return 0
}

// 提前结束会议室的使用
func EndUseRoom(msg []byte) int {
	var data Data
	_ = json.Unmarshal(msg, &data)

	// 校验权限

	room := data.RoomId + data.StartTime
	if judgePermissions(data.UserName, data.UserDepartment, "Sign", room) == false {
		log.Println("您没有此权限")
		return 1
	}

	rw.Lock()
	defer rw.Unlock()

	c := pool.Get()
	defer c.Close()


	// 删除redis Sign字段
	key := "Sign" + room
	_, err := c.Do("del", key)
	if err != nil {
		log.Println("删除Sign字段失败")
	}

	// 删除DB预约者信息
	msql := "delete from userInfo where roomid = ? and meetingStartTime = ? and name = ? and department = ?"
	_,err = db.Exec(msql, data.RoomId, data.StartTime, data.UserName, data.UserDepartment)
	if err != nil {
		log.Println("删除mysql记录失败")
	}

	// 从Signlink删除
	node := SignMap[room]
	node.delNode()

	log.Println("提前结束会议室的使用, roomid = ", data.RoomId, " starttime = " , data.StartTime, " 用户 = ", data.UserName)
	return 0
}

// 将最新状态更新到Mysql中
func UpdateData(data *Data) {
	NowStatus := QueryRoomStatus(data)
	if NowStatus == FREE {
		db.Exec("delete from userInfo where roomId = ? and meetingStartTime = ?", data.RoomId, data.StartTime)
	} else if NowStatus == NOTSIGN {
		db.Exec("update userInfo SET status = 'notSign' where roomid = ? and meetingStartTime = ?", data.RoomId, data.StartTime)
	} else if NowStatus == USING {
		db.Exec("update userInfo SET status = 'Sign' where roomid = ? and meetingStartTime = ?", data.RoomId, data.StartTime)
	}
}

// 超时未签入或者超出使用时间
func TimeoutChangeStatus() {
	// 遍历两条链表，更新数据
	NotSignCur := NotSignHead.next
	for NotSignCur != nil {
		temp := NewData()
		temp.RoomId = NotSignCur.roomId
		temp.StartTime = NotSignCur.meetStartTime
		UpdateData(temp)
		if NotSignCur.next == nil { // 如果是最后一个节点
			NotSignCur.delNode()
			break
		}
		NotSignCur = NotSignCur.next
		NotSignCur.prev.delNode()
	}

	SignCur := SignHead.next
	for SignCur != nil {
		temp := NewData()
		temp.RoomId = SignCur.roomId
		temp.StartTime = SignCur.meetStartTime
		UpdateData(temp)
		if SignCur.next == nil { // 如果是最后一个节点
			SignCur.delNode()
			break
		}
		SignCur = SignCur.next
		SignCur.prev.delNode()
	}
}