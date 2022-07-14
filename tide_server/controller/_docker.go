package controller

import (
	"context"
	"encoding/json"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"io"
	"os"
	"text/template"
	"time"
)

const (
	containerWorkingDir = "/rtkrcv"
)

type rtkConf struct {
	Sid     string
	X, Y, Z json.Number
}

var (
	dockerCli     *client.Client
	dockerTimeout = 5 * time.Second
)

func init() {
	var err error
	dockerCli, err = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		logger.Fatal(err.Error())
	}
}
func editContainer(ctx context.Context, conf rtkConf) error {
	err := makeFile(conf)
	if err != nil {
		return err
	}
	err = dockerCli.ContainerRestart(ctx, conf.Sid, &dockerTimeout)
	if errdefs.IsNotFound(err) {
		err = makeContainer(ctx, conf.Sid)
	}
	return err
}
func removeContainer(ctx context.Context, name string) error {
	err := dockerCli.ContainerRemove(ctx, name, types.ContainerRemoveOptions{Force: true})
	if err != nil && !errdefs.IsNotFound(err) {
		return err
	}
	return os.RemoveAll(workingDir + name)
}
func makeContainer(ctx context.Context, name string) error {
	resp, err := dockerCli.ContainerCreate(ctx,
		&container.Config{
			Image:      rtkRcvImageName,
			Cmd:        []string{"rtkrcv", "-s"},
			WorkingDir: containerWorkingDir,
			Tty:        true,
			OpenStdin:  true,
		},
		&container.HostConfig{
			RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
			Mounts:        []mount.Mount{{Type: "bind", Source: workingDir + name, Target: containerWorkingDir}},
		},
		&network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{networkName: {}},
		},
		name,
	)
	if err != nil {
		return err
	}
	if err := dockerCli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return err
	}
	return nil
}
func makeFile(conf rtkConf) error {
	tmpl, err := template.ParseFiles("template/rtkrcv.conf")
	if err != nil {
		return err
	}
	// 处理文件
	err = os.MkdirAll(workingDir+conf.Sid, 0777)
	if err != nil {
		return err
	}
	navSrc, err := os.Open("template/rtkrcv.nav")
	if err != nil {
		return err
	}
	defer func() { _ = navSrc.Close() }()
	navDst, err := os.Create(workingDir + conf.Sid + "/rtkrcv.nav")
	if err != nil {
		return err
	}
	defer func() { _ = navDst.Close() }()
	_, err = io.Copy(navDst, navSrc)
	if err != nil {
		return err
	}
	// 配置文件
	confFile, err := os.Create(workingDir + conf.Sid + "/rtkrcv.conf")
	if err != nil {
		return err
	}
	defer func() { _ = confFile.Close() }()
	err = tmpl.Execute(confFile, conf)
	if err != nil {
		return err
	}
	return nil
}

//func validate(r *http.Request) (bool, error) {
//	// 检查token
//	token, err := r.Cookie("token")
//	if err != nil {
//		return false, nil
//	}
//	resp, err := httpClient.PostForm("https://test.navi-tech.net/api/dms/validate/1.0", url.Values{"token": {token.Value}})
//	if err != nil {
//		log.Println(err)
//		return false, err
//	}
//	b, err := ioutil.ReadAll(resp.Body)
//	_ = resp.Body.Close() // 读完关闭
//	if err != nil {
//		log.Println(err)
//		return false, err
//	}
//	var v api.validate
//	if err = json.Unmarshal(b, &v); err != nil {
//		log.Println(err)
//		return false, err
//	}
//	if !v.Active {
//		return false, err
//	}
//	return true, err
//}
