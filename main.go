package main

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"log"
	"meetingBook/model"
	"net/http"
	"time"
)

var allRoomId map[string]bool

func main() {
	router := gin.Default()

	allRoomId = make(map[string]bool)
	err := model.GetAllRoomInfo(allRoomId)
	if err != nil {
		log.Println(err.Error())
		return
	}

	log.Println("所有的会议室信息：")
	for key, _ := range allRoomId {
		fmt.Println(key)
	}

	go model.NotifyOutput()	// 接收[成功|失败]的消息并处理

	go func () {		// 更新脏数据
		for {
			model.UpdateDirtyData()
			time.Sleep(100 * time.Millisecond)
		}
	} ()

	router.GET("/index.html", handleIndex)

	// 预约会议室
	router.POST("/bookmeet/:roomid/:startTime", handlerBook)

	// 取消预约会议室
	router.POST("/cancelmeet/:roomid/:startTime", handlerUnBook)

	// 签入会议室
	router.POST("/signmeet/:roomid/:startTime", handlerSign)

	// 结束使用会议室
	router.POST("/stopmeet/:roomid/:startTime", handlerStopUse)

	router.Run(":8080")
}

func handleIndex (c *gin.Context) {
	c.HTML(http.StatusOK, "./templates/index.html", 1)
}

func handlerBook (c *gin.Context) {
	// 会议信息
	startTime      := c.Param("startTime")
	roomId         := c.Param("roomid")
	// 预约者信息
	username 	   := c.PostForm("name")
	userphone 	   := c.PostForm("phone")
	useremail 	   := c.PostForm("email")
	userdepartment := c.PostForm("department")
	userremark     := c.PostForm("remark")
	meetingTopic   := c.PostForm("topic")

	data := model.Data {
		RoomId         : roomId,
		UserName       : username,
		UserPhone      : userphone,
		UserEmail      : useremail,
		StartTime      : startTime,
		UserRemark     : userremark,
		MeetingTopic   : meetingTopic,
		UserDepartment : userdepartment,
	}

	if !existRoomId (&data) {
		c.String(http.StatusOK, "输入的会议室不存在")
		return
	}

	tempid, err := model.BookPrepare(&data)
	if err != nil {
		c.String(http.StatusOK, err.Error())
		return
	}
	c.String(http.StatusOK, "预约成功，请等待邮件确认")
	go  model.BookRoom(&data, tempid)
}

func handlerUnBook (c *gin.Context) {
	roomid         := c.Param("roomid")
	startTime      := c.Param("startTime")

	username       := c.PostForm("name")
	userdepartment := c.PostForm("department")

	data := model.Data {
		RoomId	       : roomid,
		StartTime	   : startTime,
		UserName	   : username,
		UserDepartment : userdepartment,
	}

	if !existRoomId (&data) {
		c.String(http.StatusOK, "输入的会议室不存在")
		return
	}

	tempid, err := model.CancelPrepare(&data)
	if err != nil {
		c.String(http.StatusOK, err.Error())
		return
	}
	c.String(http.StatusOK, "取消预约成功")
	go model.CancelRoom(&data, tempid)
}

func handlerSign (c *gin.Context) {
	roomid 	       := c.Param("roomid")
	startTime 	   := c.Param("startTime")

	username 	   := c.PostForm("name")
	userdepartment := c.PostForm("department")

	data := model.Data {
		RoomId         : roomid,
		StartTime      : startTime,
		UserName       : username,
		UserDepartment : userdepartment,
	}

	if !existRoomId (&data) {
		c.String(http.StatusOK, "输入的会议室不存在")
		return
	}

	tempid, err := model.SignPrepare(&data)
	if err != nil {
		c.String(http.StatusOK, err.Error())
		return
	}
	c.String(http.StatusOK, "签入成功")
	go model.SignRoom(&data, tempid)
}

func handlerStopUse (c *gin.Context) {
	roomid         := c.Param("roomid")
	startTime      := c.Param("startTime")

	username       := c.PostForm("name")
	userdepartment := c.PostForm("department")

	data := model.Data {
		RoomId	       : roomid,
		StartTime	   : startTime,
		UserName	   : username,
		UserDepartment : userdepartment,
	}

	if !existRoomId (&data) {
		c.String(http.StatusOK, "输入的会议室不存在")
		return
	}

	tempid, err := model.StopPrepare(&data)
	if err != nil {
		c.String(http.StatusOK, err.Error())
		return
	}
	c.String(http.StatusOK, "结束使用成功")
	go model.StopRoom(&data, tempid)
}

func existRoomId (data *model.Data) bool {
	_, ok := allRoomId[data.RoomId]
	return ok
}

/*
之前是同步的去处理，现在改成异步的，先校验权限等，如果成功使用一个协程去写入DB。但是如果在写入DB的过程中出了问题该如何解决。
暂时解决方案：先返回的提示语句写成"预约完成，请留意邮件检查是否成功"，在协程中将发生的错误信息写入ABORT，然后开启另外一个协程用IO多路复用的方式
监听abort和success两个管道，传输的数据包括data和结果，然后根据结果来向其发送邮件通知最终结果。
 */