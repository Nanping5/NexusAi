package image

import (
	mylogger "NexusAi/pkg/logger"
	"bufio"
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	_ "image/gif"
	"os"
	"path/filepath"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
	"golang.org/x/image/draw"
)

type ImageRecognizer struct {
	session      *ort.Session[float32] //oonx
	inputName    string
	outputName   string
	inputH       int
	inputW       int
	labels       []string
	inputTensor  *ort.Tensor[float32]
	outputTensor *ort.Tensor[float32]
}

const (
	defaultInputName  = "data"
	defaultOutputName = "mobilenetv20_output_flatten0_reshape0"

	// ImageNet 标准化参数
	meanR = 0.485
	meanG = 0.456
	meanB = 0.406
	stdR  = 0.229
	stdG  = 0.224
	stdB  = 0.225
)

var (
	initOnce sync.Once
	initErr  error
)

// NewImageRecognizer 创建一个新的 ImageRecognizer 实例
func NewImageRecognizer(modelPath string, labelPath string, inputH int, inputW int) (*ImageRecognizer, error) {
	if inputH <= 0 || inputW <= 0 {
		inputH = 224
		inputW = 224
	}

	//初始化ONNX
	initOnce.Do(func() {
		// macOS 需要设置正确的库路径
		if dylibPath := os.Getenv("ONNXRUNTIME_LIB_PATH"); dylibPath != "" {
			ort.SetSharedLibraryPath(dylibPath)
		} else if _, err := os.Stat("/opt/homebrew/lib/libonnxruntime.dylib"); err == nil {
			// Apple Silicon Mac Homebrew 路径
			ort.SetSharedLibraryPath("/opt/homebrew/lib/libonnxruntime.dylib")
		} else if _, err := os.Stat("/usr/local/lib/libonnxruntime.dylib"); err == nil {
			// Intel Mac Homebrew 路径
			ort.SetSharedLibraryPath("/usr/local/lib/libonnxruntime.dylib")
		}
		initErr = ort.InitializeEnvironment()
	})
	if initErr != nil {
		mylogger.Logger.Error("Failed to initialize ONNX Runtime environment: " + initErr.Error())
		return nil, initErr
	}

	inputShape := ort.NewShape(1, 3, int64(inputH), int64(inputW))
	inData := make([]float32, inputShape.FlattenedSize())
	inTensor, err := ort.NewTensor(inputShape, inData)
	if err != nil {
		mylogger.Logger.Error("Failed to create input tensor: " + err.Error())
		return nil, err
	}
	outShape := ort.NewShape(1, 1000)
	outTensor, err := ort.NewEmptyTensor[float32](outShape)
	if err != nil {
		inTensor.Destroy()
		mylogger.Logger.Error("Failed to create output tensor: " + err.Error())
		return nil, err
	}

	//创建Session

	session, err := ort.NewSession[float32](modelPath,
		[]string{defaultInputName},
		[]string{defaultOutputName},
		[]*ort.Tensor[float32]{inTensor},
		[]*ort.Tensor[float32]{outTensor},
	)
	if err != nil {
		inTensor.Destroy()
		outTensor.Destroy()
		mylogger.Logger.Error("Failed to create ONNX Runtime session: " + err.Error())
		return nil, err
	}
	labels, err := loadLabels(labelPath)
	if err != nil {
		session.Destroy()
		inTensor.Destroy()
		outTensor.Destroy()
		mylogger.Logger.Error("Failed to load labels: " + err.Error())
		return nil, err
	}

	return &ImageRecognizer{
		session:      session,
		inputName:    defaultInputName,
		outputName:   defaultOutputName,
		inputH:       inputH,
		inputW:       inputW,
		labels:       labels,
		inputTensor:  inTensor,
		outputTensor: outTensor,
	}, nil
}

// Close 释放资源
func (r *ImageRecognizer) Close() {
	if r.session != nil {
		_ = r.session.Destroy()
		r.session = nil
	}
	if r.inputTensor != nil {
		_ = r.inputTensor.Destroy()
		r.inputTensor = nil
	}
	if r.outputTensor != nil {
		_ = r.outputTensor.Destroy()
		r.outputTensor = nil
	}
}

// loadLabels 从文件加载标签列表
func loadLabels(labelPath string) ([]string, error) {
	file, err := os.Open(filepath.Clean(labelPath))
	if err != nil {
		mylogger.Logger.Error("Failed to open label file: " + err.Error())
		return nil, err
	}

	defer file.Close()

	var labels []string
	// 逐行读取标签文件
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			labels = append(labels, line)
		}
	}

	if err := scanner.Err(); err != nil {
		mylogger.Logger.Error("Failed to read label file: " + err.Error())
		return nil, err
	}

	return labels, nil

}

// PredictFromFile 从指定路径的图像文件进行预测
func (r *ImageRecognizer) PredictFromFile(imagePath string) (string, error) {
	// 预处理图像并填充输入张量
	file, err := os.Open(filepath.Clean(imagePath))
	if err != nil {
		mylogger.Logger.Error("Failed to open image file: " + err.Error())
		return "", err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		mylogger.Logger.Error("Failed to decode image: " + err.Error())
		return "", err
	}
	return r.PredictFromImage(img)
}

// PredictFromImage 直接从 image.Image 进行预测，适用于内存中的图像数据
func (r *ImageRecognizer) PredictFromImage(img image.Image) (string, error) {
	// 预处理图像并填充输入张量
	reSizedImg := image.NewNRGBA(image.Rect(0, 0, int(r.inputH), int(r.inputW)))

	draw.CatmullRom.Scale(reSizedImg, reSizedImg.Bounds(), img, img.Bounds(), draw.Over, nil)

	h, w := r.inputH, r.inputW
	ch := 3 //RGB

	data := make([]float32, h*w*ch)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			rgb := reSizedImg.At(x, y)
			rVal, g, b, _ := rgb.RGBA()
			// 归一化到 [0, 1]
			rNorm := float32(rVal>>8) / 255.0
			gNorm := float32(g>>8) / 255.0
			bNorm := float32(b>>8) / 255.0
			// 应用 ImageNet 标准化: (x - mean) / std
			data[y*w+x] = (rNorm - meanR) / stdR
			data[h*w+y*w+x] = (gNorm - meanG) / stdG
			data[2*h*w+y*w+x] = (bNorm - meanB) / stdB
		}

	}
	inData := r.inputTensor.GetData()
	copy(inData, data)

	// 运行推理
	if err := r.session.Run(); err != nil {
		mylogger.Logger.Error("Failed to run inference: " + err.Error())
		return "", err
	}
	outData := r.outputTensor.GetData()

	// 找到最大概率的索引
	maxIdx := 0
	for i := 1; i < len(outData); i++ {
		if outData[i] > outData[maxIdx] {
			maxIdx = i
		}
	}

	if maxIdx >= len(r.labels) || maxIdx < 0 {
		mylogger.Logger.Error("Predicted index exceeds label list length")
		return "Unknown", nil
	}

	return r.labels[maxIdx], nil
}

func (r *ImageRecognizer) PredictFromBuffer(buf []byte) (string, error) {
	reader := bytes.NewReader(buf)
	img, _, err := image.Decode(reader)
	if err != nil {
		// 尝试手动解码常见格式
		reader.Reset(buf)
		if img, err = jpeg.Decode(reader); err != nil {
			reader.Reset(buf)
			if img, err = png.Decode(reader); err != nil {
				mylogger.Logger.Error("Failed to decode image from buffer: " + err.Error())
				return "", err
			}
		}
	}
	return r.PredictFromImage(img)
}
