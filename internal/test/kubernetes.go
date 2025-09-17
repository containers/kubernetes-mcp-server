package test

import (
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func KubeConfigFake() *clientcmdapi.Config {
	fakeConfig := clientcmdapi.NewConfig()
	fakeConfig.Clusters["fake"] = clientcmdapi.NewCluster()
	fakeConfig.Clusters["fake"].Server = "https://127.0.0.1:6443"
	fakeConfig.Clusters["additional-cluster"] = clientcmdapi.NewCluster()
	fakeConfig.AuthInfos["fake"] = clientcmdapi.NewAuthInfo()
	fakeConfig.AuthInfos["additional-auth"] = clientcmdapi.NewAuthInfo()
	fakeConfig.Contexts["fake-context"] = clientcmdapi.NewContext()
	fakeConfig.Contexts["fake-context"].Cluster = "fake"
	fakeConfig.Contexts["fake-context"].AuthInfo = "fake"
	fakeConfig.Contexts["additional-context"] = clientcmdapi.NewContext()
	fakeConfig.Contexts["additional-context"].Cluster = "additional-cluster"
	fakeConfig.Contexts["additional-context"].AuthInfo = "additional-auth"
	fakeConfig.CurrentContext = "fake-context"
	return fakeConfig
}
