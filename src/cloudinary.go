package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"time"
)

// uploadClient 与 SEO 共用带 DNS 备用线路的 Transport，超时放宽到 60s。
var uploadClient = &http.Client{Timeout: 60 * time.Second, Transport: externalTransport}

// uploadToCloudinary 通过 unsigned upload 上传图片到 Cloudinary。
func uploadToCloudinary(c CloudConfig, filename string, data []byte) (string, string, error) {
	if c.CloudName == "" || c.UploadPreset == "" {
		return "", "", fmt.Errorf("请先在设置中填写 cloudName 与 uploadPreset")
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return "", "", err
	}
	if _, err := fw.Write(data); err != nil {
		return "", "", err
	}
	_ = mw.WriteField("upload_preset", c.UploadPreset)
	if err := mw.Close(); err != nil {
		return "", "", err
	}

	endpoint := fmt.Sprintf("https://api.cloudinary.com/v1_1/%s/image/upload", url.PathEscape(c.CloudName))
	req, err := http.NewRequest(http.MethodPost, endpoint, &body)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := uploadClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("网络请求失败：%w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var out struct {
		SecureURL string `json:"secure_url"`
		PublicID  string `json:"public_id"`
		Error     struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", "", fmt.Errorf("图床返回异常（HTTP %d）", resp.StatusCode)
	}
	if out.SecureURL == "" {
		msg := out.Error.Message
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return "", "", fmt.Errorf("%s", msg)
	}
	return out.SecureURL, out.PublicID, nil
}
