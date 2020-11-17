package main

import (
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gomodule/redigo/redis"
	"meetingBook/model"
)
//
//var db *sql.DB

//func init() {
//	var err error
//	//db, err = sql.Open("mysql", "root@tcp(127.0.0.1:3306)/GinMeetRoom")
//	db, err = sql.Open("mysql", "root:Xtm_0124@tcp(cdb-027nnpt2.cd.tencentcdb.com:10106)/GinMeetRoom")
//
//	if err != nil {
//		panic(err)
//	}
//}
var pool *redis.Pool  //创建redis连接池

func init() {
	pool = &redis.Pool{     //实例化一个连接池
		MaxIdle:16,    //最初的连接数量
		MaxActive:0,    //连接池最大连接数量,不确定可以用0（0表示自动定义），按需分配
		IdleTimeout:300,    //连接关闭时间 300秒 （300秒不使用自动关闭）
		Dial: func() (redis.Conn ,error){     //要连接的redis数据库
			return redis.Dial("tcp","127.0.0.1:6379")
		},
	}
}

func main() {

	//test := "allRoom1234567890"
	//
	//str := test[7:]
	//fmt.Println(str)

	str := model.QueryAllFreeRoom()
	for k, v := range str {
		//if v == "" {
		//	continue
		//}
		fmt.Println("i = ", k , " v = " , v)
	}

	//model.BookRoom("20201112120000","111","xtm","123456789","1522972330@qq.com","bytedance","12","toopic")
	////model.CancelRoom("20201112120000", "111", "xtm", "bytedance")
	//model.SignRoom("111", "20201112120000", "xtm", "bytedance")
	//model.EndUseRoom("xtm","bytedance","111","20201112120000")




	//c := pool.Get()
	//flag, _ := redis.String(c.Do("hget", "notSign", "111202011121200"))
	//fmt.Println(flag)


	//msql := "insert into userInfo(name, phone, email, department, topic, roomId, meetingStartTime, remark, status, bookTime ) VALUES( ?, ?, ?, ?, ?, ?, ?, ? , 'notSign' , now() )"
	//DB, _ := sql.Open("mysql", "root:Xtm_0124@tcp(cdb-027nnpt2.cd.tencentcdb.com:10106)/GinMeetRoom")
// insert into userInfo(name, phone, email, department, topic, roomId, meetingStartTime, remark, status, bookTime) VALUES('xtm', '123456789', '1522972330@qq.com', 'bytedance', 'topic', '123', '202011121200', 'nonoe', 'notSign', now());

	//msql := "insert into  userInfo( roomId, status ) VALUES( ?, 'notSign' )"
	//_, err := DB.Exec(msql, "999")
	//_, err := DB.Exec(msql, "xtm", "123456789", "1522972330@qq.com", "bytedance", "topic", "123", "202011121200", "none")
	//if err != nil {
	//	log.Println("预约会议室插入mysql失败")
	//}
	//book := make(map[string]bool, 100)
	//book["test11"] = true
	//book["test1"] = true
	//book["test12"] = true
	//book["here"] = true
	//
	//c, _ := redis.Dial("tcp", "127.0.0.1:6379")
	//
	//resKeys, err := redis.Values(c.Do("hkeys", "notSign"))
	//if err != nil {
	//	fmt.Println("hkeys failed", err.Error())
	//} else {
	//	fmt.Printf("myhash's keys is :\n")
	//	for _, v := range resKeys {
	//		//fmt.Printf("%s \n", v.([]byte))
	//		book[string(v.([]byte))] = false
	//	}
	//}
	//
	//for str, flag := range book {
	//	if flag == false {
	//		continue
	//	}
	//	fmt.Println(str)
	//}
	//
	//c.Close()
	//
	//c, _ := redis.Dial("tcp", "127.0.0.1:6379")
	//
	//res, _ := redis.String(c.Do("hget", "notSign", "test12"))
	//
	//fmt.Println(res)



	//formatTimeStr :="20201110210000"
	//
	//formatTime,err:=time.Parse("20060102150405",formatTimeStr)
	//
	//if err==nil{
	//
	//	fmt.Println(formatTime) //打印结果：2017-04-11 13:33:37 +0000 UTC
	//	fmt.Println(formatTime.Unix()-8*60*60)
	//}



	//c.Close()


	// "select roomId, meetingStartTime from userInfo where status = notSign or status = Sign"

	//_, _ = sql.Open("mysql", "root:Xtm_0124@tcp(cdb-027nnpt2.cd.tencentcdb.com:10106)/GinMeetRoom")
	//temp := "20000104"
	//t := "notSign"

	//_, err := db.Exec("update userInfo SET status = ? where roomId = 1 and meetingStartTime = ?", t, temp)
	//_, err := db.Exec("update userInfo SET status = " + t + " where roomId = 1 and meetingStartTime = " + temp)
	//if err != nil {
	//	log.Println("failed")
	//}

	//rows, _ := db.Query("select roomId, meetingStartTime, status from userInfo where status = ? or status = ?", "notSign", "Sign")
	//
	//defer func() {
	//	if rows != nil {
	//		rows.Close()
	//	}
	//}()
	//
	//for rows.Next() {
	//	var roomId, starttime ,status string
	//	if err := rows.Scan(&roomId, &starttime, &status); err != nil {
	//		log.Fatal( err)
	//	}
	//	room := roomId + starttime
	//	NowStatus := queryRoomStatus(room)
	//	if NowStatus == FREE {
	//		db.Exec("delete from userInfo where roomId = ? and meetingStartTime = ?", roomId, starttime)
	//	} else if NowStatus == NOTSIGN {
	//		db.Exec("update userInfo SET status = 'notSign' where roomid = ? and meetingStartTime = ?", roomId, starttime)
	//	} else if NowStatus == USING {
	//		db.Exec("update userInfo SET status = 'Sign' where roomid = ? and meetingStartTime = ?", roomId, starttime)
	//	}
	//	//fmt.Printf("%s %s %s \n", roomId, starttime, status)
	//}
	//if err := rows.Err(); err != nil {
	//	log.Fatal(err)
	//}

	//queryRoomStatus()
/*
   name, phone, email, department, topic, roomId, meetingStartTime, remark, status, bookTime

create table userInfo (
	uid INT PRIMARY KEY AUTO_INCREMENT,
	name VARCHAR(100),
	phone VARCHAR(15),
	email VARCHAR(50),
	department VARCHAR(100),
	topic VARCHAR(140),
	roomId VARCHAR(50) NOT NULL,
	meetingStartTime VARCHAR(30),
	remark VARCHAR(300),
	status VARCHAR(10),
	bookTime VARCHAR(50),
	INDEX idx_roomid (roomId)
);


   insert into userInfo(name, phone, email, department, topic, roomId, meetingStartTime, remark, status, bookTime) VALUES('xtm','123','qq.com', 'department', 'topic', '11', '20000104', '1','notSign',now())

*/




}
