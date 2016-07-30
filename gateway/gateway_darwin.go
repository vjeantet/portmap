// +build darwin

package gateway

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
)

func getGatewayAddrs() (gwaddr []net.IP, err error) {
	routeCmd := exec.Command("/sbin/route", "-n", "get", "0.0.0.0")
	output, err := routeCmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	// Darwin route out format is always like this:
	//    route to: default
	// destination: default
	//        mask: default
	//     gateway: 192.168.1.1
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "gateway:" {
			ip := net.ParseIP(fields[1])
			if ip != nil {
				gwaddr = append(gwaddr, ip)
				return gwaddr, nil
			}
		}
	}

	return nil, fmt.Errorf("No gateway")

}
