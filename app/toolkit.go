package app

import (
	"os"

	"github.com/suisrc/zgg/z"
)

var (
	namespace_ = ""

	C = struct {
	}{}
)

// func init() {
// 	zc.Register(&C)
// 	z.Register("11-app.init", func(zgg *z.Zgg) z.Closed {
// 		// 创建 k8sclient
// 		client, err := CreateClient(z.C.Server.Local)
// 		if err != nil {
// 			zgg.ServeStop("create k8s client error: ", err.Error()) // 初始化失败，直接退出
// 			return nil
// 		}
// 		// z.RegSvc(zgg.GetSvcKit(), client)
// 		z.Println("create k8s client success: local=", z.C.Server.Local)
// 		zgg.SvcKit.Set("k8sclient", client) // 注册 k8sclient
// 		return nil
// 	})
// }

// -----------------------------------------------------------------------

// 获取当前命名空间 k8s namespace
func K8sNS() string {
	if namespace_ != "" {
		return namespace_
	}
	ns, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		z.Printf("unable to read namespace: %s, return 'default'", err.Error())
		namespace_ = "default"
	} else {
		namespace_ = string(ns)
	}
	return namespace_
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
// 		z.Println("using local kubeconfig.")
// 		kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
// 		return clientcmd.BuildConfigFromFlags("", kubeconfig)
// 	}
// 	z.Println("using in cluster kubeconfig.")
// 	return rest.InClusterConfig()
// }
