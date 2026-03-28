package router

import (
	imageController "NexusAi/controller/image"

	"github.com/gin-gonic/gin"
)

func RegisterImageRouter(r *gin.RouterGroup) {
	// 图片识别接口
	r.POST("/recognize", imageController.RecognizeImage)
}
