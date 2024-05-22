package process

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/containerd/containerd/v2/pkg/bw"
	"github.com/containerd/containerd/v2/pkg/oci"
)

type initState interface {
	Start(context.Context) error
	Delete(context.Context) error
	Kill(context.Context, uint32, bool) error
	SetExited(int)
	Status(context.Context) (string, error)
}

type createdState struct {
	p *Init

	status string
}

func (s *createdState) Start(ctx context.Context) error {
	s.status = "running"

	if _, err := os.Stat(filepath.Join(s.p.Rootfs, "model")); err == nil {
		exePath, err := os.Executable()
		if err != nil {
			return err
		}
		rootPath := filepath.Dir(exePath)

		// time.Sleep(30 * time.Second)

		args := []string{
			"serve",
			"--rootfs", s.p.Rootfs,
		}
		spec, err := oci.ReadSpec(filepath.Join(s.p.Bundle, oci.ConfigFilename))
		if err != nil {
			return err
		}

		sbName := spec.Annotations["io.kubernetes.cri.sandbox-name"]

		cmd := exec.Command(filepath.Join(rootPath, "runm"), args...)
		cmd.Env = os.Environ()
		// XXX: SET OLLAMA_HOST
		data := strings.NewReader(fmt.Sprintf(`{ "name": "%s" }`, sbName))
		req, err := http.NewRequest("GET", "http://localhost:9090/", data)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}

			get := bw.Response{}
			err = json.Unmarshal(body, &get)
			if err != nil {
				return err
			}

			cmd.Env = append(cmd.Env, fmt.Sprintf("OLLAMA_HOST=127.0.0.1:%d", get.HostPort))
		}

		err = cmd.Start()
		if err == nil {
			s.p.pid = cmd.Process.Pid
		}
		return err
	}
	return nil
}

func (s *createdState) Delete(ctx context.Context) error {
	s.status = "deleted"
	return nil
}

func (s *createdState) Kill(ctx context.Context, sig uint32, all bool) error {
	if s.status == "stopped" {
		return nil
	}
	s.status = "stopped"
	return s.p.kill(ctx, sig, all)
}

func (s *createdState) SetExited(status int) {
	s.p.setExited(status)
	s.status = "stopped"
}

func (s *createdState) Status(ctx context.Context) (string, error) {
	return s.status, nil
}
