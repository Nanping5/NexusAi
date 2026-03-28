package service

import (
	"NexusAi/common/image"
	"NexusAi/config"
	mylogger "NexusAi/pkg/logger"
	"context"
	"io"
	"mime/multipart"
	"sync"
)

type imageService struct {
	recognizer *image.ImageRecognizer
	once       sync.Once
	initErr    error
}

var ImageService = &imageService{}

// InitRecognizer 初始化图像识别器（应在服务启动时调用）
func (s *imageService) InitRecognizer() error {
	s.once.Do(func() {
		modelPath := config.GetConfig().ImageRecognitionConfig.ModelPath
		labelsPath := config.GetConfig().ImageRecognitionConfig.LabelPath
		inputH, inputW := 224, 224

		s.recognizer, s.initErr = image.NewImageRecognizer(modelPath, labelsPath, inputH, inputW)
		if s.initErr != nil {
			mylogger.Logger.Error("Failed to initialize ImageRecognizer: " + s.initErr.Error())
		}
	})
	return s.initErr
}

func (s *imageService) RecognizeImage(ctx context.Context, file *multipart.FileHeader) (string, error) {
	// 确保识别器已初始化
	if err := s.InitRecognizer(); err != nil {
		return "", err
	}

	src, err := file.Open()
	if err != nil {
		mylogger.Logger.Error("Failed to open uploaded file: " + err.Error())
		return "", err
	}
	defer src.Close()

	buf, err := io.ReadAll(src)
	if err != nil {
		mylogger.Logger.Error("Failed to read image file: " + err.Error())
		return "", err
	}
	return s.recognizer.PredictFromBuffer(buf)
}

func (s *imageService) RecognizeFromBuffer(ctx context.Context, buf []byte) (string, error) {
	// 确保识别器已初始化
	if err := s.InitRecognizer(); err != nil {
		return "", err
	}
	return s.recognizer.PredictFromBuffer(buf)
}
