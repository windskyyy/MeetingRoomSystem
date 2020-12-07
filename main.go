package main

import (
	"github.com/gin-gonic/gin"
	"meetingBook/model"
	"net/http"
)


func main() {
	router := gin.Default()

	router.GET("/bookmeet.html", handleIndex)

	// 预约会议室
	router.POST("/bookmeet/:roomid/:setartTime", handleBook)

	// 取消预约会议室
	router.POST("/cancelmeet/:roomid/:startTime", handleUnbook)

	// 签入会议室
	router.POST("/signmeet/:roomid/:starttime", handleSign)

	// 结束使用会议室
	router.POST("/endmeeed/:roomid/:starttime", handleUnuse)

	router.Run(":8080")
}

func handleIndex (c *gin.Context) {
	c.HTML(http.StatusOK, "index.html", 1)
}

func handleBook (c *gin.Context) {
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
		RoomId:         roomId,
		UserName:       username,
		UserPhone:      userphone,
		UserEmail:      useremail,
		StartTime:      startTime,
		UserRemark:     userremark,
		MeetingTopic:   meetingTopic,
		UserDepartment: userdepartment,
	}


	err := model.BookRoom(&data)

	if err != nil {
		c.String(http.StatusOK, err.Error())
	} else {
		c.String(http.StatusOK, "预约成功")
	}
}

func handleUnbook (c *gin.Context) {
	roomid := c.Param("roomid")
	startTime := c.Param("startTime")

	username := c.PostForm("name")
	userdepartment := c.PostForm("department")

	data := model.Data {
		RoomId: roomid,
		StartTime: startTime,
		UserName: username,
		UserDepartment: userdepartment,
	}

	err := model.CancelRoom(&data)

	if err != nil {
		c.String(http.StatusOK, err.Error())
	} else {
		c.String(http.StatusOK, "取消预约成功")
	}
}

func handleSign (c *gin.Context) {
	roomid := c.Param("roomid")
	startTime := c.Param("startTime")

	username := c.PostForm("name")
	userdepartment := c.PostForm("department")

	data := model.Data {
		RoomId: roomid,
		StartTime: startTime,
		UserName: username,
		UserDepartment: userdepartment,
	}

	err := model.SignRoom(&data)

	if err != nil {
		c.String(http.StatusOK, err.Error())
	} else {
		c.String(http.StatusOK, "签入成功")
	}
}

func handleUnuse (c *gin.Context) {
	roomid := c.Param("roomid")
	startTime := c.Param("startTime")

	username := c.PostForm("name")
	userdepartment := c.PostForm("department")

	data := model.Data {
		RoomId: roomid,
		StartTime: startTime,
		UserName: username,
		UserDepartment: userdepartment,
	}

	err := model.EndUseRoom(&data)

	if err != nil {
		c.String(http.StatusOK, err.Error())
	} else {
		c.String(http.StatusOK, "取消预约成功")
	}
}



