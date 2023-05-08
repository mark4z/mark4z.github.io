package main

import (
	"bufio"
	"os"
	"strings"
)

func main() {
	// 打开文件
	file, err := os.Open("/Users/mark4zlv/GolandProjects/mark4z.github.io/source/_posts/book-riscv-rev3-source.md")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	// 创建一个数组来存储每一行
	var lines []string

	// 逐行读取文件，去掉空行
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) > 0 {
			lines = append(lines, line)
		}
	}

	// 检查错误
	if err := scanner.Err(); err != nil {
		panic(err)
	}

	// 合并以大写字母开头的多行文本
	mergedLines := make([]string, 0)
	i := 0
	for i < len(lines) {
		currentLine := lines[i]

		// 判断当前行是否以大写字母开头
		if isUpperCase(currentLine[0]) && !endsWithPeriodOrColon(currentLine) {
			mergedLine := currentLine

			// 向下查找直到找到以.或:结尾的行
			j := i + 1
			for j < len(lines) && !endsWithPeriodOrColon(lines[j]) && !strings.HasPrefix(lines[j], "```") {
				mergedLine += " " + strings.TrimSpace(lines[j])
				j++
			}
			if endsWithPeriodOrColon(lines[j]) {
				mergedLine += " " + strings.TrimSpace(lines[j])
				j++
			}

			// 将合并后的行存储到数组中
			mergedLines = append(mergedLines, "")
			mergedLines = append(mergedLines, mergedLine)

			// 更新i的值，指向下一个需要处理的行
			i = j
		} else if isUpperCase(currentLine[0]) && endsWithPeriodOrColon(currentLine) {
			mergedLines = append(mergedLines, "")
			mergedLines = append(mergedLines, currentLine)
			i++
		} else if strings.HasPrefix(currentLine, "```") {
			mergedLines = append(mergedLines, currentLine)
			i++
			for i < len(lines) && !strings.HasPrefix(lines[i], "```") {
				mergedLines = append(mergedLines, lines[i])
				i++
			}
			mergedLines = append(mergedLines, lines[i])
			i++
		} else if strings.HasPrefix(currentLine, "|") {
			mergedLines = append(mergedLines, currentLine)
			i++
		} else {
			// 如果当前行不以大写字母开头，则直接存储到数组中
			mergedLines = append(mergedLines, "")
			mergedLines = append(mergedLines, currentLine)
			i++
		}
	}

	// 在合并后的行存储到数组中后增加一个空行
	mergedLines = append(mergedLines, "")

	// 打开文件
	target, err := os.OpenFile("/Users/mark4zlv/GolandProjects/mark4z.github.io/source/_posts/book-riscv-rev3.md", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		panic(err)
	}
	defer target.Close()
	// result按行写入target
	for _, line := range mergedLines {
		target.WriteString(line + "\n")
	}
}

// 判断一个字符是否为大写字母
func isUpperCase(c byte) bool {
	return c >= 'A' && c <= 'Z'
}

// 判断一个字符串是否以.或:结尾
func endsWithPeriodOrColon(s string) bool {
	return strings.HasSuffix(s, ".") || strings.HasSuffix(s, ":")
}
