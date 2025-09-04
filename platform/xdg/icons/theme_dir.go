package icons

import (
	"math"
	"strings"

	"gopkg.in/ini.v1"
)

type iconThemeDir struct {
	subPath          string
	scale            int
	size             int
	sizeEffective    int
	minSize          int
	minSizeEffective int
	maxSize          int
	maxSizeEffective int
}

func newIconThemeDir(subPath string, sec *ini.Section) *iconThemeDir {
	dirType := strings.ToLower(sec.Key("Type").MustString("Threshold"))
	scale := sec.Key("Scale").MustInt(1)
	size := sec.Key("Size").MustInt(-1)
	threshold := sec.Key("Threshold").MustInt(2)
	minSize := sec.Key("MinSize").MustInt(size)
	maxSize := sec.Key("MaxSize").MustInt(size)

	switch dirType {
	case "fixed":
		minSize = size
		maxSize = size
	case "threshold":
		minSize = size - threshold
		maxSize = size + threshold
	case "scalable":
		// pass
	}

	return &iconThemeDir{
		subPath:          subPath,
		scale:            scale,
		size:             size,
		sizeEffective:    size * scale,
		minSize:          minSize,
		minSizeEffective: minSize * scale,
		maxSize:          maxSize,
		maxSizeEffective: maxSize * scale,
	}
}

func (i *iconThemeDir) matchesSize(iconSize int, iconScale int) bool {
	return i.scale == iconScale && i.minSize <= iconSize && iconSize <= i.maxSize
}

func (i *iconThemeDir) sizeDistance(iconSize int, iconScale int) int {
	iconEffective := iconSize * iconScale

	if i.minSizeEffective <= iconEffective && iconEffective <= i.maxSizeEffective {
		dt := i.sizeEffective - iconEffective
		if dt < 0 {
			return -dt
		}

		return dt
	}

	return math.MaxInt64
}

func (i *iconThemeDir) sizeDistanceOutside(iconSize int, iconScale int) int {
	iconEffective := iconSize * iconScale

	if iconEffective < i.minSizeEffective {
		return i.minSizeEffective - iconEffective
	}

	if iconEffective > i.maxSizeEffective {
		return iconEffective - i.maxSizeEffective
	}

	if i.sizeEffective >= iconEffective {
		return i.sizeEffective - iconEffective
	}

	return iconEffective - i.sizeEffective
}
