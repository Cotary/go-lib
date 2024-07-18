package utils

import (
	"os"
	"path/filepath"
	"time"
)

func WriteLog(files string, data map[string]string) error {
	text := "\n" + time.Now().String() + ":\n"
	for key, val := range data {
		text += key + ": " + val + "\n"
	}
	return WriteFileAppend(files, text)
}

// WriteFileAppend 向文件追加内容，文件自动创建
func WriteFileAppend(files string, val string) error {
	err := CreateFile(files)
	if err != nil {
		return err
	}
	myFile, err := os.OpenFile(files, os.O_APPEND|os.O_WRONLY, os.ModePerm)
	if err != nil {
		return err
	}
	defer myFile.Close()

	// Write the string to the file
	_, err = myFile.WriteString(val + "\n")
	if err != nil {
		return err
	}
	return nil

}

func CreateFile(files string) error {
	//创建文件夹
	paths, _ := filepath.Split(files)
	err := CreateMultiDir(paths)
	if err != nil {
		return err
	}
	//判断文件是否存在
	if !isExist(files) {
		dstFile, err := os.Create(files)
		if err != nil {
			return err
		}
		defer dstFile.Close()
	}
	return nil
}

// CreateMultiDir  调用os.MkdirAll递归创建文件夹
func CreateMultiDir(filePath string) error {
	if !isExist(filePath) {
		err := os.MkdirAll(filePath, os.ModePerm)
		if err != nil {
			return err
		}
		return err
	}
	return nil
}

// isExist 判断所给路径文件/文件夹是否存在(返回true是存在)
func isExist(path string) bool {
	_, err := os.Stat(path) //os.Stat获取文件信息
	if err != nil {
		if os.IsExist(err) {
			return true
		}
		return false
	}
	return true
}
