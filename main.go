package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"syscall"

	"github.com/kelseyhightower/kargo"
)

var (
	hostname string
	region   string
	httpAddr string
)

func main() {
	flag.StringVar(&httpAddr, "http", "127.0.0.1:80", "HTTP service address")
	flag.Parse()

	var err error
	hostname, err = os.Hostname()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	region = os.Getenv("REGION")

	fmt.Println("Starting hello-universe...")
	errChan := make(chan error, 10)

	var dm *kargo.DeploymentManager
	if kargo.EnableKubernetes {
		// link, err := kargo.Upload(kargo.UploadConfig{
		// 	ProjectID:  "hightowerlabs",
		// 	BucketName: "hello-universe",
		// 	ObjectName: "hello-universe",
		// })
		link := ""
		config := kargo.UploadConfig{
			ProjectID:  "hightowerlabs",
			BucketName: "hello-universe",
			ObjectName: "hello-universe",
		}

		tmpDir, err := ioutil.TempDir("", "")
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		if config.Path == "" {
			fmt.Printf("Building %s binary...\n", config.ObjectName)
			output, err := build(tmpDir, config.ObjectName)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			config.Path = output
			fmt.Println("Created: " + config.Path)
			fmt.Println("Serving file")
			_, f := path.Split(output)
			link = "http://178.0.0.1:8090/" + f
			go http.ListenAndServe(":8090", http.FileServer(http.Dir(tmpDir)))
		}

		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		env := make(map[string]string)
		env["HELLO_UNIVERSE_TOKEN"] = os.Getenv("HELLO_UNIVERSE_TOKEN")

		fmt.Println("Going on with Kargo")
		dm = kargo.New()
		err = dm.Create(kargo.DeploymentConfig{
			Args:      []string{"-http=0.0.0.0:80"},
			Env:       env,
			Name:      "hello-universe",
			BinaryURL: link,
		})
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		err = dm.Logs(os.Stdout)
		if err != nil {
			fmt.Println("Local logging has been disabled.")
		}
	} else {
		http.HandleFunc("/", httpHandler)

		go func() {
			errChan <- http.ListenAndServe(httpAddr, nil)
		}()
	}

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	for {
		select {
		case err := <-errChan:
			if err != nil {
				fmt.Printf("%s - %s\n", hostname, err)
				os.Exit(1)
			}
		case <-signalChan:
			fmt.Printf("%s - Shutdown signal received, exiting...\n", hostname)
			if kargo.EnableKubernetes {
				err := dm.Delete()
				if err != nil {
					fmt.Printf("%s - %s\n", hostname, err)
					os.Exit(1)
				}
			}
			os.Exit(0)
		}
	}
}

func build(tmpDir, name string) (string, error) {
	output := filepath.Join(tmpDir, name)

	ldflags := `-extldflags "-static"`
	command := []string{
		"go", "build", "-o", output, "-a", "--ldflags",
		ldflags, "-tags", "netgo",
		"-installsuffix", "netgo", ".",
	}
	cmd := exec.Command(command[0], command[1:]...)

	gopath := os.Getenv("GOPATH")
	goroot := os.Getenv("GOROOT")
	path := os.Getenv("PATH")
	cmd.Env = []string{
		"GOOS=linux",
		"GOARCH=amd64",
		"GOPATH=" + gopath,
		"GOROOT=" + goroot,
		"PATH=" + path,
	}

	data, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println(string(data))
		return "", err
	}

	return output, nil
}
