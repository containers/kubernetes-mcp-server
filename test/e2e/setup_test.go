//go:build e2e

package e2e

import (
	"fmt"
	"os/exec"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func clientsetFromKubeconfig(kubeconfig string) (kubernetes.Interface, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("build rest config: %w", err)
	}
	return kubernetes.NewForConfig(config)
}

func runHelm(helmBin, kubeconfig string, args ...string) error {
	allArgs := make([]string, 0, len(args)+2)
	allArgs = append(allArgs, "--kubeconfig", kubeconfig)
	allArgs = append(allArgs, args...)
	cmd := exec.Command(helmBin, allArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %w\n%s", helmBin, args, err, string(out))
	}
	return nil
}
