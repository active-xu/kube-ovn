package util

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
	"k8s.io/klog"

	kubeovnv1 "github.com/kubeovn/kube-ovn/pkg/apis/kubeovn/v1"
)

// GenerateMac generates mac address.
func GenerateMac() string {
	prefix := "00:00:00"
	b := make([]byte, 3)
	_, err := rand.Read(b)
	if err != nil {
		klog.Errorf("generate mac error: %v", err)
	}

	mac := fmt.Sprintf("%s:%02X:%02X:%02X", prefix, b[0], b[1], b[2])
	return mac
}

func Ip2BigInt(ipStr string) *big.Int {
	ipBigInt := big.NewInt(0)
	if CheckProtocol(ipStr) == kubeovnv1.ProtocolIPv4 {
		ipBigInt.SetBytes(net.ParseIP(ipStr).To4())
	} else {
		ipBigInt.SetBytes(net.ParseIP(ipStr).To16())
	}
	return ipBigInt
}

func BigInt2Ip(ipInt *big.Int) string {
	buf := make([]byte, 4)
	if len(ipInt.Bytes()) > 4 {
		buf = make([]byte, 16)
	}
	ip := net.IP(ipInt.FillBytes(buf))
	return ip.String()
}

func SubnetNumber(subnet string) string {
	_, cidr, _ := net.ParseCIDR(subnet)
	return cidr.IP.String()
}

func SubnetBroadcast(subnet string) string {
	_, cidr, _ := net.ParseCIDR(subnet)
	var length uint
	if CheckProtocol(subnet) == kubeovnv1.ProtocolIPv4 {
		length = 32
	} else {
		length = 128
	}
	maskLength, _ := cidr.Mask.Size()
	ipInt := Ip2BigInt(cidr.IP.String())
	size := big.NewInt(0).Lsh(big.NewInt(1), length-uint(maskLength))
	size = big.NewInt(0).Sub(size, big.NewInt(1))
	return BigInt2Ip(ipInt.Add(ipInt, size))
}

func FirstIP(subnet string) (string, error) {
	_, cidr, err := net.ParseCIDR(subnet)
	if err != nil {
		return "", fmt.Errorf("%s is not a valid cidr", subnet)
	}
	ipInt := Ip2BigInt(cidr.IP.String())
	return BigInt2Ip(ipInt.Add(ipInt, big.NewInt(1))), nil
}

func LastIP(subnet string) (string, error) {
	_, cidr, err := net.ParseCIDR(subnet)
	if err != nil {
		return "", fmt.Errorf("%s is not a valid cidr", subnet)
	}
	var length uint
	if CheckProtocol(subnet) == kubeovnv1.ProtocolIPv4 {
		length = 32
	} else {
		length = 128
	}
	maskLength, _ := cidr.Mask.Size()
	ipInt := Ip2BigInt(cidr.IP.String())
	size := big.NewInt(0).Lsh(big.NewInt(1), length-uint(maskLength))
	size = big.NewInt(0).Sub(size, big.NewInt(2))
	return BigInt2Ip(ipInt.Add(ipInt, size)), nil
}

func CIDRConflict(a, b string) bool {
	for _, cidra := range strings.Split(a, ",") {
		for _, cidrb := range strings.Split(b, ",") {
			if CheckProtocol(cidra) != CheckProtocol(cidrb) {
				continue
			}
			aIp, aIpNet, aErr := net.ParseCIDR(cidra)
			bIp, bIpNet, bErr := net.ParseCIDR(cidrb)
			if aErr != nil || bErr != nil {
				return false
			}
			if aIpNet.Contains(bIp) || bIpNet.Contains(aIp) {
				return true
			}
		}
	}
	return false
}

func CIDRContainIP(cidrStr, ipStr string) bool {
	var containFlag bool
	for _, cidr := range strings.Split(cidrStr, ",") {
		_, cidrNet, err := net.ParseCIDR(cidr)
		if err != nil {
			return false
		}

		for _, ip := range strings.Split(ipStr, ",") {
			if CheckProtocol(cidr) != CheckProtocol(ip) {
				continue
			}
			ipAddr := net.ParseIP(ip)
			if ipAddr == nil {
				return false
			}

			if cidrNet.Contains(ipAddr) {
				containFlag = true
			} else {
				containFlag = false
			}
		}
	}
	// v4 and v6 address should be both matched for dualstack check
	return containFlag
}

func CheckProtocol(address string) string {
	ips := strings.Split(address, ",")
	if len(ips) == 2 {
		v4IP := net.ParseIP(strings.Split(ips[0], "/")[0])
		v6IP := net.ParseIP(strings.Split(ips[1], "/")[0])
		if v4IP.To4() != nil && v6IP.To16() != nil {
			return kubeovnv1.ProtocolDual
		}
	}

	address = strings.Split(address, "/")[0]
	ip := net.ParseIP(address)
	if ip.To4() != nil {
		return kubeovnv1.ProtocolIPv4
	}
	return kubeovnv1.ProtocolIPv6
}

// ProtocolToFamily converts protocol string to netlink family
func ProtocolToFamily(protocol string) (int, error) {
	switch protocol {
	case kubeovnv1.ProtocolDual:
		return unix.AF_UNSPEC, nil
	case kubeovnv1.ProtocolIPv4:
		return unix.AF_INET, nil
	case kubeovnv1.ProtocolIPv6:
		return unix.AF_INET6, nil
	default:
		return -1, fmt.Errorf("invalid protocol: %s", protocol)
	}
}

func AddressCount(network *net.IPNet) float64 {
	prefixLen, bits := network.Mask.Size()
	if bits-prefixLen < 2 {
		return 0
	}
	return math.Pow(2, float64(bits-prefixLen)) - 2
}

func GenerateRandomV4IP(cidr string) string {
	if len(strings.Split(cidr, "/")) != 2 {
		return ""
	}
	ip := strings.Split(cidr, "/")[0]
	netMask, _ := strconv.Atoi(strings.Split(cidr, "/")[1])
	hostNum := 32 - netMask
	add, err := rand.Int(rand.Reader, big.NewInt(1<<(uint(hostNum)-1)))
	if err != nil {
		klog.Fatalf("failed to generate random ip, %v", err)
	}
	t := big.NewInt(0).Add(Ip2BigInt(ip), add)
	return fmt.Sprintf("%s/%d", BigInt2Ip(t), netMask)
}

func IPToString(ip string) string {
	ipNet, _, err := net.ParseCIDR(ip)
	if err == nil {
		return ipNet.String()
	}
	ipNet = net.ParseIP(ip)
	if ipNet != nil {
		return ipNet.String()
	}
	return ""
}

func IsValidIP(ip string) bool {
	return net.ParseIP(ip) != nil
}

func CheckCidrs(cidr string) error {
	for _, cidrBlock := range strings.Split(cidr, ",") {
		if _, _, err := net.ParseCIDR(cidrBlock); err != nil {
			return errors.New("CIDRInvalid")
		}
	}
	return nil
}

func GetGwByCidr(cidrStr string) (string, error) {
	var gws []string
	for _, cidr := range strings.Split(cidrStr, ",") {
		gw, err := FirstIP(cidr)
		if err != nil {
			return "", err
		}
		gws = append(gws, gw)
	}

	return strings.Join(gws, ","), nil
}

func AppendGwByCidr(gateway, cidrStr string) (string, error) {
	var gws []string
	for _, cidr := range strings.Split(cidrStr, ",") {
		if CheckProtocol(gateway) == CheckProtocol(cidr) {
			gws = append(gws, gateway)
			continue
		} else {
			gw, err := FirstIP(cidr)
			if err != nil {
				return "", err
			}
			var gwArray [2]string
			if CheckProtocol(gateway) == kubeovnv1.ProtocolIPv4 {
				gwArray[0] = gateway
				gwArray[1] = gw
			} else {
				gwArray[0] = gw
				gwArray[1] = gateway
			}
			gws = gwArray[:]
		}
	}

	return strings.Join(gws, ","), nil
}

func SplitIpsByProtocol(excludeIps []string) ([]string, []string) {
	var v4ExcludeIps, v6ExcludeIps []string
	for _, ex := range excludeIps {
		ips := strings.Split(ex, "..")
		if len(ips) == 1 {
			if net.ParseIP(ips[0]).To4() != nil {
				v4ExcludeIps = append(v4ExcludeIps, ips[0])
			} else {
				v6ExcludeIps = append(v6ExcludeIps, ips[0])
			}
		} else {
			if net.ParseIP(ips[0]).To4() != nil {
				v4ExcludeIps = append(v4ExcludeIps, ex)
			} else {
				v6ExcludeIps = append(v6ExcludeIps, ex)
			}
		}
	}

	return v4ExcludeIps, v6ExcludeIps
}

func GetStringIP(v4IP, v6IP string) string {
	var ipStr string
	if IsValidIP(v4IP) && IsValidIP(v6IP) {
		ipStr = v4IP + "," + v6IP
	} else if IsValidIP(v4IP) {
		ipStr = v4IP
	} else if IsValidIP(v6IP) {
		ipStr = v6IP
	}
	return ipStr
}

func GetIpAddrWithMask(ip, cidr string) string {
	var ipAddr string
	if CheckProtocol(cidr) == kubeovnv1.ProtocolDual {
		cidrBlocks := strings.Split(cidr, ",")
		ips := strings.Split(ip, ",")
		v4IP := fmt.Sprintf("%s/%s", ips[0], strings.Split(cidrBlocks[0], "/")[1])
		v6IP := fmt.Sprintf("%s/%s", ips[1], strings.Split(cidrBlocks[1], "/")[1])
		ipAddr = v4IP + "," + v6IP
	} else {
		ipAddr = fmt.Sprintf("%s/%s", ip, strings.Split(cidr, "/")[1])
	}
	return ipAddr
}

func GetIpWithoutMask(ipStr string) string {
	var ips []string
	for _, ip := range strings.Split(ipStr, ",") {
		ips = append(ips, strings.Split(ip, "/")[0])
	}
	return strings.Join(ips, ",")
}

func SplitStringIP(ipStr string) (string, string) {
	var v4IP, v6IP string
	if CheckProtocol(ipStr) == kubeovnv1.ProtocolDual {
		v4IP = strings.Split(ipStr, ",")[0]
		v6IP = strings.Split(ipStr, ",")[1]
	} else if CheckProtocol(ipStr) == kubeovnv1.ProtocolIPv4 {
		v4IP = ipStr
	} else if CheckProtocol(ipStr) == kubeovnv1.ProtocolIPv6 {
		v6IP = ipStr
	}

	return v4IP, v6IP
}

// ExpandExcludeIPs used to get exclude ips in range of subnet cidr, excludes cidr addr and broadcast addr
func ExpandExcludeIPs(excludeIPs []string, cidr string) []string {
	rv := []string{}
	for _, excludeIP := range excludeIPs {
		if strings.Contains(excludeIP, "..") {
			parts := strings.Split(excludeIP, "..")
			if len(parts) != 2 || CheckProtocol(parts[0]) != CheckProtocol(parts[1]) {
				klog.Errorf("invalid exlcude IP: %s", excludeIP)
				continue
			}
			s := Ip2BigInt(parts[0])
			e := Ip2BigInt(parts[1])
			if s.Cmp(e) > 0 {
				continue
			}

			for _, cidrBlock := range strings.Split(cidr, ",") {
				if CheckProtocol(cidrBlock) != CheckProtocol(parts[0]) {
					continue
				}

				firstIP, err := FirstIP(cidrBlock)
				if err != nil {
					klog.Error(err)
					continue
				}
				if firstIP == SubnetBroadcast(cidrBlock) {
					klog.Errorf("no available IP address in CIDR %s", cidrBlock)
					continue
				}
				lastIP, _ := LastIP(cidrBlock)
				s1, e1 := s, e
				if s1.Cmp(Ip2BigInt(firstIP)) < 0 {
					s1 = Ip2BigInt(firstIP)
				}
				if e1.Cmp(Ip2BigInt(lastIP)) > 0 {
					e1 = Ip2BigInt(lastIP)
				}
				if c := s1.Cmp(e1); c == 0 {
					rv = append(rv, BigInt2Ip(s1))
				} else if c < 0 {
					rv = append(rv, BigInt2Ip(s1)+".."+BigInt2Ip(e1))
				}
			}
		} else {
			for _, cidrBlock := range strings.Split(cidr, ",") {
				if CIDRContainIP(cidrBlock, excludeIP) && excludeIP != SubnetNumber(cidrBlock) && excludeIP != SubnetBroadcast(cidrBlock) {
					rv = append(rv, excludeIP)
					break
				}
			}
		}
	}
	klog.V(3).Infof("expand exclude ips %v", rv)
	return rv
}

func ContainsIPs(excludeIP string, ip string) bool {
	if strings.Contains(excludeIP, "..") {
		parts := strings.Split(excludeIP, "..")
		s := Ip2BigInt(parts[0])
		e := Ip2BigInt(parts[1])
		ipv := Ip2BigInt(ip)
		if s.Cmp(ipv) <= 0 && e.Cmp(ipv) >= 0 {
			return true
		}
	} else {
		if excludeIP == ip {
			return true
		}
	}
	return false
}

func CountIpNums(excludeIPs []string) float64 {
	var count float64
	for _, excludeIP := range excludeIPs {
		if strings.Contains(excludeIP, "..") {
			var val big.Int
			parts := strings.Split(excludeIP, "..")
			s := Ip2BigInt(parts[0])
			e := Ip2BigInt(parts[1])
			v, _ := new(big.Float).SetInt(val.Add(val.Sub(e, s), big.NewInt(1))).Float64()
			count += v
		} else {
			count++
		}
	}
	return count
}

func GatewayContains(gatewayNodeStr, gateway string) bool {
	// the format of gatewayNodeStr can be like 'kube-ovn-worker:172.18.0.2, kube-ovn-control-plane:172.18.0.3', which consists of node name and designative egress ip
	for _, gw := range strings.Split(gatewayNodeStr, ",") {
		if strings.Contains(gw, ":") {
			gw = strings.TrimSpace(strings.Split(gw, ":")[0])
		} else {
			gw = strings.TrimSpace(gw)
		}
		if gw == gateway {
			return true
		}
	}
	return false
}
