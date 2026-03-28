# 图像识别模块 API 文档

> 基础路径: `/api/v1/image`
> 版本: v1.0.0
> 更新日期: 2026-03-28

---

## 目录

- [概述](#概述)
- [接口列表](#接口列表)
  - [图像识别](#1-图像识别)
- [模型说明](#模型说明)

---

## 概述

图像识别模块基于 ONNX Runtime 和 MobileNetV2 模型，提供图像分类识别功能。

**认证方式**: JWT Bearer Token（**所有接口都需要认证**）

**请求头:**

```
Authorization: Bearer <token>
```

**支持的图像格式:**
- JPEG
- PNG
- GIF
- BMP
- TIFF

**图像处理:**
- 自动缩放至 224x224
- RGB 通道归一化
- 输出 1000 类 ImageNet 分类结果

---

## 接口列表

### 1. 图像识别

上传图像并获取分类识别结果。

| 项目 | 内容 |
|------|------|
| URL | `/api/v1/image/recognize` |
| Method | `POST` |
| 认证 | **需要 JWT 认证** |
| Content-Type | `multipart/form-data` |

**请求参数 (Form-Data):**

| 参数名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| image | file | 是 | 图像文件，支持 JPEG/PNG/GIF/BMP/TIFF |

**请求示例:**

```bash
POST /api/v1/image/recognize
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
Content-Type: multipart/form-data

--boundary
Content-Disposition: form-data; name="image"; filename="cat.jpg"
Content-Type: image/jpeg

<二进制图像数据>
--boundary--
```

**cURL 示例:**

```bash
curl -X POST "http://localhost:8080/api/v1/image/recognize" \
  -H "Authorization: Bearer <token>" \
  -F "image=@/path/to/image.jpg"
```

**成功响应:**

```json
{
    "code": 1000,
    "msg": "success",
    "data": {
        "class_name": "Egyptian cat"
    }
}
```

**失败响应:**

```json
// 未登录
{
    "code": 2007,
    "msg": "用户未登录"
}

// 服务繁忙（文件上传失败或识别失败）
{
    "code": 4001,
    "msg": "服务繁忙"
}

// 无效Token
{
    "code": 2006,
    "msg": "无效的Token"
}
```

**说明:**
- 模型使用 MobileNetV2，支持 1000 种 ImageNet 分类
- 返回概率最高的分类名称
- 图像会自动缩放至 224x224 进行识别
- 支持常见的图像格式

---

## 模型说明

### MobileNetV2

- **模型类型**: 轻量级卷积神经网络
- **输入尺寸**: 224 x 224 x 3 (RGB)
- **输出**: 1000 类 ImageNet 分类
- **推理引擎**: ONNX Runtime

### 支持的分类类别

模型支持 ImageNet 的 1000 个分类，包括但不限于：

- 动物：猫、狗、鸟类、鱼类等
- 植物：各种花卉、树木
- 交通工具：汽车、飞机、船等
- 日常物品：家具、电子设备、食物等
- 自然场景：山川、海滩、天空等

完整分类列表参考：`common/image/labels/imagenet_labels.txt`

### 识别精度

- 对常见物体识别准确率较高
- 对于细节相似的可能产生误判
- 建议上传清晰、主体明确的图像

---

## 配置说明

### config.toml 配置

```toml
[image_recognition_config]
model_path = "./common/image/models/mobilenetv2.onnx"
label_path = "./common/image/labels/imagenet_labels.txt"
```

### 环境变量

| 变量名 | 说明 |
|--------|------|
| `ONNXRUNTIME_LIB_PATH` | ONNX Runtime 库路径（可选） |
