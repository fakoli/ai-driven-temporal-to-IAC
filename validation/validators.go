package validation

import (
	"net"
	"regexp"
	"strconv"
	"strings"
)

// RFC 1918 private address ranges
var (
	private10  = net.IPNet{IP: net.ParseIP("10.0.0.0"), Mask: net.CIDRMask(8, 32)}
	private172 = net.IPNet{IP: net.ParseIP("172.16.0.0"), Mask: net.CIDRMask(12, 32)}
	private192 = net.IPNet{IP: net.ParseIP("192.168.0.0"), Mask: net.CIDRMask(16, 32)}
)

// AWS region pattern
var awsRegionPattern = regexp.MustCompile(`^(us|eu|ap|sa|ca|me|af)-(north|south|east|west|central|northeast|southeast)-[1-3]$`)

// IsRFC1918 checks if a CIDR or IP is within RFC 1918 private address space
func IsRFC1918(addr string) bool {
	ip := extractIP(addr)
	if ip == nil {
		return false
	}

	return private10.Contains(ip) || private172.Contains(ip) || private192.Contains(ip)
}

// IsCIDR validates CIDR notation
func IsCIDR(cidr string) bool {
	_, _, err := net.ParseCIDR(cidr)
	return err == nil
}

// IsValidIP validates IPv4 address format
func IsValidIP(addr string) bool {
	// Remove CIDR suffix if present
	if idx := strings.Index(addr, "/"); idx != -1 {
		addr = addr[:idx]
	}
	ip := net.ParseIP(addr)
	return ip != nil && ip.To4() != nil
}

// CIDRContains checks if the first CIDR contains the second IP or CIDR
func CIDRContains(container, contained string) bool {
	_, containerNet, err := net.ParseCIDR(container)
	if err != nil {
		return false
	}

	// Check if contained is an IP or CIDR
	containedIP := extractIP(contained)
	if containedIP == nil {
		return false
	}

	return containerNet.Contains(containedIP)
}

// CIDRsOverlap checks if any CIDRs in the two slices overlap
func CIDRsOverlap(cidrs1, cidrs2 []string) bool {
	for _, c1 := range cidrs1 {
		_, net1, err1 := net.ParseCIDR(c1)
		if err1 != nil {
			continue
		}
		for _, c2 := range cidrs2 {
			_, net2, err2 := net.ParseCIDR(c2)
			if err2 != nil {
				continue
			}
			if net1.Contains(net2.IP) || net2.Contains(net1.IP) {
				return true
			}
		}
	}
	return false
}

// CIDRSize returns the prefix length of a CIDR (e.g., 16 for /16)
func CIDRSize(cidr string) int {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return -1
	}
	ones, _ := network.Mask.Size()
	return ones
}

// CIDRHostCount returns the number of usable hosts in a CIDR
// For AWS, 5 IPs are reserved per subnet
func CIDRHostCount(cidr string) int {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return 0
	}
	ones, bits := network.Mask.Size()
	totalIPs := 1 << (bits - ones)

	// Subtract network address and broadcast
	if totalIPs <= 2 {
		return 0
	}
	return totalIPs - 2
}

// IsValidAWSRegion checks if the region is a valid AWS region format
func IsValidAWSRegion(region string) bool {
	return awsRegionPattern.MatchString(region)
}

// IsValidEKSVersion checks if the version is a supported EKS version
func IsValidEKSVersion(version string) bool {
	// Supported versions as of 2024: 1.28, 1.29, 1.30, 1.31
	supportedVersions := map[string]bool{
		"1.28": true,
		"1.29": true,
		"1.30": true,
		"1.31": true,
	}
	return supportedVersions[version]
}

// IsValidClusterName validates EKS cluster naming conventions
func IsValidClusterName(name string) bool {
	if len(name) < 1 || len(name) > 100 {
		return false
	}
	// Must start with letter, contain only lowercase alphanumeric and hyphens
	pattern := regexp.MustCompile(`^[a-z][a-z0-9-]*[a-z0-9]$`)
	if len(name) == 1 {
		return regexp.MustCompile(`^[a-z]$`).MatchString(name)
	}
	return pattern.MatchString(name)
}

// ParseCIDRPrefix extracts the prefix length from a CIDR string
func ParseCIDRPrefix(cidr string) (int, error) {
	parts := strings.Split(cidr, "/")
	if len(parts) != 2 {
		return 0, net.InvalidAddrError("invalid CIDR")
	}
	return strconv.Atoi(parts[1])
}

// extractIP extracts the IP address from a string that may be an IP or CIDR
func extractIP(addr string) net.IP {
	// Try parsing as CIDR first
	ip, _, err := net.ParseCIDR(addr)
	if err == nil {
		return ip
	}

	// Try parsing as plain IP
	ip = net.ParseIP(addr)
	if ip != nil {
		return ip.To4()
	}

	return nil
}

// ValidateNodeSizing validates EKS node group sizing constraints
func ValidateNodeSizing(minSize, desiredSize, maxSize int) bool {
	return minSize >= 1 && minSize <= desiredSize && desiredSize <= maxSize
}

// ContainsString checks if a string slice contains a value
func ContainsString(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

// IsSubnetOfVPC checks if a subnet CIDR is within a VPC CIDR
func IsSubnetOfVPC(subnetCIDR, vpcCIDR string) bool {
	return CIDRContains(vpcCIDR, subnetCIDR)
}

// AllCIDRsRFC1918 checks if all CIDRs in a slice are RFC 1918 compliant
func AllCIDRsRFC1918(cidrs []string) bool {
	for _, cidr := range cidrs {
		if !IsRFC1918(cidr) {
			return false
		}
	}
	return true
}

// ValidateCIDRRange checks if a CIDR prefix is within a valid range
func ValidateCIDRRange(cidr string, minPrefix, maxPrefix int) bool {
	prefix := CIDRSize(cidr)
	if prefix < 0 {
		return false
	}
	return prefix >= minPrefix && prefix <= maxPrefix
}
