package utils

import (
	"fmt"
	"regexp"
	"strings"
)

func RegexpMatchUrlFromString(str string) (string, error) {
	urlReg, err := regexp.Compile(`https?://[\w.-]+[\w/-]*[\w.-:]*\??[\w=&:\-+%.]*/*`)
	if err != nil {
		return "", fmt.Errorf("match url regexp compile error: %s", err.Error())
	}

	findStr := urlReg.FindString(str)
	if len(findStr) <= 0 {
		return "", fmt.Errorf("str not have url")
	}

	return findStr, nil
}

// ExtractBetweenFirstAndLastBraces 提取第一个 { 和最后一个 } 之间的内容
func ExtractBetweenFirstAndLastBraces(input string) string {
	// 查找第一个 { 的位置
	firstOpenBrace := strings.Index(input, "{")
	if firstOpenBrace == -1 {
		return ""
	}

	// 查找最后一个 } 的位置
	lastCloseBrace := strings.LastIndex(input, "}")
	if lastCloseBrace == -1 {
		return ""
	}

	// 提取内容
	return input[firstOpenBrace : lastCloseBrace+1]
}

func ParseJsonWithLevel(str string, level int) string {
	for i := 0; i < level; i++ {
		str = strings.ReplaceAll(str, `\"`, `"`)
	}
	return str
}
