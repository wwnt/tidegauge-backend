package camera

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"tide/pkg/net/digest"
)

const (
	// snapshotTimeout 覆盖"请求 + 摘要认证重试 + 读完图片"的全过程。摄像头接受连接后
	// 不响应时，没有它调用方 goroutine 会永久挂住。
	snapshotTimeout = 20 * time.Second
	// maxSnapshotBytes 是单张快照的上限，防止异常响应把内存吃光。
	maxSnapshotBytes = 32 << 20
)

// OnvifSnapshot 抓取一张摄像头快照。任何非 200 响应都作为错误返回——
// 以前返回 (nil, nil)，上层会把"密码错了/地址不对"当成一张空图片成功转发。
func OnvifSnapshot(url string, username, password string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), snapshotTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	var transport http.RoundTripper
	if username == "" {
		transport = http.DefaultTransport
	} else {
		transport = digest.NewTransport(username, password)
	}
	resp, err := transport.RoundTrip(req)
	if resp != nil {
		// 摘要认证失败时 transport 可能同时返回响应和错误，两种情况都要关 body。
		defer func() { _ = resp.Body.Close() }()
	}
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("camera snapshot: unexpected status %s", resp.Status)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSnapshotBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxSnapshotBytes {
		return nil, fmt.Errorf("camera snapshot: larger than %d bytes", maxSnapshotBytes)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("camera snapshot: empty response body")
	}
	return data, nil
}
