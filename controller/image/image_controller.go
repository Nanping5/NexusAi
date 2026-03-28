package imageController

import (
	ret "NexusAi/common/code"
	response "NexusAi/common/response/common"
	"NexusAi/service"

	"github.com/gin-gonic/gin"
)

func RecognizeImage(c *gin.Context) {
	file, err := c.FormFile("image")
	if err != nil {
		response.Fail(c, ret.CodeServerBusy)
		return
	}

	className, err := service.ImageService.RecognizeImage(c.Request.Context(), file)
	if err != nil {
		response.Fail(c, ret.CodeServerBusy)
		return
	}
	response.Success(c, gin.H{"class_name": className})
}
