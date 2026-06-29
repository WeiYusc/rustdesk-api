package admin

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/response"
	"github.com/lejianwen/rustdesk-api/v2/lib/upload"
)

type File struct {
}

// OssToken 文件
// @Tags 文件
// @Summary 获取ossToken
// @Description 获取ossToken
// @Accept  json
// @Produce  json
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/file/oss_token [get]
// @Security token
func (f *File) OssToken(c *gin.Context) {
	token := global.Oss.GetPolicyToken("")
	response.Success(c, token)
}

type FileBack struct {
	upload.CallbackBaseForm
	Url string `json:"url"`
}

const bytesPerMegabyte = 1024 * 1024

// Notify 上传成功后回调
func (f *File) Notify(c *gin.Context) {

	res := global.Oss.Verify(c.Request)
	if !res {
		response.Fail(c, 101, response.TranslateMsg(c, "NoAccess"))
		return
	}
	fm := &FileBack{}
	if err := c.ShouldBind(fm); err != nil {
		fmt.Println(err)
	}
	fm.Url = global.Config.Oss.Host + "/" + fm.Filename
	response.Success(c, fm)

}

// Upload 上传文件到本地
// @Tags 文件
// @Summary 上传文件到本地
// @Description 上传文件到本地
// @Accept  multipart/form-data
// @Produce  json
// @Param file formData file true "上传文件示例"
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /admin/file/upload [post]
// @Security token
func (f *File) Upload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError")+err.Error())
		return
	}
	if file.Size <= 0 || file.Size > uploadMaxBytes() {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	if filepath.Base(file.Filename) != file.Filename || strings.Contains(file.Filename, "..") || strings.ContainsAny(file.Filename, `/\\`) {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	src, err := file.Open()
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	defer src.Close()
	buffer := make([]byte, 512)
	n, err := io.ReadFull(src, buffer)
	if err != nil && err != io.ErrUnexpectedEOF {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	mimeType := http.DetectContentType(buffer[:n])
	extByMime := map[string]string{
		"image/jpeg": ".jpg",
		"image/png":  ".png",
		"image/webp": ".webp",
	}
	ext, ok := extByMime[mimeType]
	if !ok {
		response.Fail(c, 101, response.TranslateMsg(c, "ParamsError"))
		return
	}
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	timePath := time.Now().Format("20060102") + "/"
	webPath := "/upload/avatar/" + timePath
	path := filepath.Join(global.Config.Gin.ResourcesPath, "public", "upload", "avatar", time.Now().Format("20060102"))
	if err := os.MkdirAll(path, 0755); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	randomName, err := randomHex(16)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	filename := randomName + ext
	dst := filepath.Join(path, filename)
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	defer out.Close()
	if _, err := io.Copy(out, src); err != nil {
		response.Fail(c, 101, response.TranslateMsg(c, "OperationFailed")+err.Error())
		return
	}
	response.Success(c, gin.H{
		"url": webPath + filename,
	})
}

func uploadMaxBytes() int64 {
	maxSizeMb := global.Config.App.UploadMaxSizeMb
	if maxSizeMb <= 0 {
		maxSizeMb = 10
	}
	return maxSizeMb * bytesPerMegabyte
}

func randomHex(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
