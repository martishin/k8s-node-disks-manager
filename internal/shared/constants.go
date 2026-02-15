package shared

import (
	"fmt"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	Group   = "localtest.example.com"
	Version = "v1alpha1"
	Kind    = "NodeDisk"

	LabelNode = "localtest.example.com/node"
	LabelDisk = "localtest.example.com/disk"

	LabelAutoReserve = "localtest.example.com/auto-reserve"

	DefaultDiskDirContainer = "/host-node-disks-manager/disks"
	DefaultHostRootPath     = "/var/lib/node-disks-manager"
	DefaultContainerRoot    = "/host-node-disks-manager"
)

var (
	GVR = schema.GroupVersionResource{Group: Group, Version: Version, Resource: "nodedisks"}
	GVK = schema.GroupVersionKind{Group: Group, Version: Version, Kind: Kind}
)

var invalidNameChars = regexp.MustCompile(`[^a-z0-9-]`)

func NodeDiskName(nodeName, diskID string) string {
	normNode := normalizeName(nodeName)
	normDisk := normalizeName(diskID)
	name := fmt.Sprintf("node-disk-%s-%s", normNode, normDisk)
	if len(name) > 253 {
		return name[:253]
	}
	return strings.Trim(name, "-")
}

func normalizeName(in string) string {
	v := strings.ToLower(in)
	v = strings.ReplaceAll(v, "_", "-")
	v = strings.ReplaceAll(v, ".", "-")
	v = invalidNameChars.ReplaceAllString(v, "-")
	v = strings.Trim(v, "-")
	if v == "" {
		return "unknown"
	}
	for strings.Contains(v, "--") {
		v = strings.ReplaceAll(v, "--", "-")
	}
	return v
}
