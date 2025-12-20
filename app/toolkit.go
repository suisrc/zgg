package app

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/suisrc/zgg/z"

	"errors"
)

var (
	C = struct {
		Namespace string
		Token     string
	}{}
)

// func init() {
// 	z.Register("11-app.init", func(srv z.IServer) z.Closed {
// 		// 创建 k8sclient
// 		client, err := CreateClient(z.C.Server.Local)
// 		if err != nil {
// 			klog.Error("create k8s client error: ", err.Error())
// 			srv.ServeStop() // 初始化失败，直接退出
// 			return nil
// 		}
// 		// z.RegSvc(srv.GetSvcKit(), client)
// 		klog.Info("create k8s client success: local=", z.C.Server.Local)
// 		srv.GetSvcKit().Set("k8sclient", client) // 注册 k8sclient
// 		return nil
// 	})
// }

// -----------------------------------------------------------------------

// 获取当前命名空间 k8s namespace
func K8sNs() string {
	if C.Namespace != "" {
		return C.Namespace
	}
	ns, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		z.Printf("unable to read namespace: %s, return 'default'", err.Error())
		C.Namespace = "default"
	} else {
		C.Namespace = string(ns)
	}
	return C.Namespace
}

// // CreateClient Create the server
// func CreateClient(local bool) (*kubernetes.Clientset, error) {
// 	cfg, err := BuildConfig(local)
// 	if err != nil {
// 		return nil, errors.Wrapf(err, "error setting up cluster config")
// 	}
// 	return kubernetes.NewForConfig(cfg)
// }

// // BuildConfig Build the config
// func BuildConfig(local bool) (*rest.Config, error) {
// 	if local {
// 		klog.Info("using local kubeconfig.")
// 		kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
// 		return clientcmd.BuildConfigFromFlags("", kubeconfig)
// 	}
// 	klog.Info("using in cluster kubeconfig.")
// 	return rest.InClusterConfig()
// }

// -----------------------------------------------------------------------

func PostJson(req *http.Request) error {
	if req.Method != http.MethodPost {
		return fmt.Errorf("wrong http verb. got %s", req.Method)
	}
	if req.Body == nil {
		return errors.New("empty body")
	}
	contentType := req.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		return fmt.Errorf("wrong content type. expected 'application/json', got: '%s'", contentType)
	}
	return nil
}
