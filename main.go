package main

import (
	"github.com/gin-gonic/gin"
	"meetingBook/model"
	"net/http"
)

func main() {
	router := gin.Default()

	router.GET("/bookmeet.html", func (c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", 1)
	})

	// 预约会议室
	router.POST("/bookmeet/:roomid/:startTime", func (c *gin.Context) {
		// 会议信息
		startTime := c.Param("startTime")
		roomId    := c.Param("roomid")
		// 预约者信息
		username := c.PostForm("name")
		userphone := c.PostForm("phone")
		useremail := c.PostForm("email")
		userdepartment := c.PostForm("department")
		userremark := c.PostForm("remark")
		meetingTopic := c.PostForm("topic")

		ok := model.BookRoom(startTime, roomId, username, userphone, useremail, userdepartment, userremark, meetingTopic)
		if ok == 0 {
			c.String(http.StatusOK, "预约成功")
		} else if ok == 1 {
			c.String(http.StatusOK, "目标时间的该会议室已经被占用,预约失败")
		} else if ok == 2 {
			c.String(http.StatusOK, "预约会议室插入redis失败,预约失败")
		} else if ok == 3 {
			c.String(http.StatusOK, "预约会议室插入mysql失败，预约失败")
		}
	})

	// 取消预约会议室
	router.POST("/cancelmeet/:roomid/:startTime", func (c *gin.Context) {
		roomid := c.Param("roomid")
		startTime := c.Param("startTime")

		username := c.PostForm("name")
		userdepartment := c.PostForm("department")

		ok := model.CancelRoom(startTime, roomid, username, userdepartment)
		if ok == 0 {
			c.String(http.StatusOK, "取消预约成功")
		} else if ok == 1 {
			c.String(http.StatusOK, "会议室没有被预约")
		} else if ok == 2 {
			c.String(http.StatusOK, "您无权限做此操作")
		}
	})

	// 签入会议室
	router.POST("/signmeet/:roomid/:starttime", func (c *gin.Context) {
		roomid := c.Param("roomid")
		startTime := c.Param("startTime")

		username := c.PostForm("name")
		userdepartment := c.PostForm("department")

		ok := model.SignRoom(roomid, startTime, username, userdepartment)
		if ok == 0 {
			c.String(http.StatusOK, "取消预约成功")
		} else if ok == 1 {
			c.String(http.StatusOK, "会议室没有被预约")
		} else if ok == 2 {
			c.String(http.StatusOK, "您无权限做此操作")
		}
	})

	// 结束使用会议室
	router.POST("/endmeeed/:roomid/:starttime", func (c *gin.Context) {
		roomid := c.Param("roomid")
		startTime := c.Param("startTime")

		username := c.PostForm("name")
		userdepartment := c.PostForm("department")

		ok := model.EndUseRoom(username, userdepartment, roomid, startTime)
		if ok == 0 {
			c.String(http.StatusOK, "取消预约成功")
		} else if ok == 1 {
			c.String(http.StatusOK, "您无权限做此操作")
		}
	})

	router.Run(":8080")
}



