package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/Unknwon/log"
	"net"
	"os"
	"runtime"
	"strings"
)

const (
	UploadCommond      = "upload"
	DownloadCommond    = "download"
	ShowCommond        = "show"
	SearchCommond      = "search"
	OperationSecOffset = 16
	FileNameSecOffset  = 256 + OperationSecOffset
	FileSizeSecOffset  = 8 + FileNameSecOffset
)

var DownloadDir string
var IPPort string

func Int32ToBytes(n int32) []byte {
	tmp := int32(n)
	bytesBuffer := bytes.NewBuffer([]byte{})
	binary.Write(bytesBuffer, binary.BigEndian, tmp)
	return bytesBuffer.Bytes()
}

func Int64ToBytes(n int64) []byte {
	tmp := int64(n)
	bytesBuffer := bytes.NewBuffer([]byte{})
	binary.Write(bytesBuffer, binary.BigEndian, tmp)
	return bytesBuffer.Bytes()
}

func BytesToInt32(b []byte) int32 {
	bytesBuffer := bytes.NewBuffer(b)
	var tmp int32
	binary.Read(bytesBuffer, binary.BigEndian, &tmp)
	return int32(tmp)
}

func BytesToInt64(b []byte) int64 {
	bytesBuffer := bytes.NewBuffer(b)
	var tmp int64
	binary.Read(bytesBuffer, binary.BigEndian, &tmp)
	return int64(tmp)
}

func GetValidByte(src []byte) []byte {
	var str_buf []byte
	for _, v := range src {
		if v != 0 {
			str_buf = append(str_buf, v)
		} else {
			break
		}
	}
	return str_buf
}

func GetDir(fullPath string) string {
	var lastIndex int = 0
	if runtime.GOOS == "windows" {
		lastIndex = strings.LastIndex(fullPath, "\\")
	} else {
		lastIndex = strings.LastIndex(fullPath, "/")
	}

	runes := []rune(fullPath)
	l := lastIndex
	if lastIndex > len(runes) {
		l = len(runes)
	}
	return string(runes[0 : l+1])
}

func uploadFile(filePath string, done chan bool) error {
	defer func() {
		done <- true
	}()

	fileInfo, err := os.Stat(filePath)
	if err != nil {
		log.Error("This file not exist! " + err.Error())
		return err
	}

	if fileInfo.IsDir() {
		log.Error("This is a directory!")
		return nil
	}

	var conn net.Conn
	conn, err = net.Dial("tcp", IPPort)
	if err != nil {
		log.Error("Dial fail! " + err.Error())
		return err
	}

	defer conn.Close()

	buf := make([]byte, 1024)

	//file size and name 16 + 256 + 8
	copy(buf[0:], []byte(UploadCommond))
	copy(buf[OperationSecOffset:], []byte(fileInfo.Name()))
	copy(buf[FileNameSecOffset:], Int64ToBytes(fileInfo.Size()))

	//fmt.Println(buf)

	_, err = conn.Write(buf)
	if err != nil {
		log.Error(err.Error())
		return err
	}

	//file content
	var f *os.File
	f, err = os.Open(filePath)
	if err != nil {
		log.Error(err.Error())
		return err
	}

	defer f.Close()

	var oneFileSectionSize int64 = 1024
	var fileSectionCount int64 = 0
	fileSize := fileInfo.Size()

	if fileSize%oneFileSectionSize != 0 {
		fileSectionCount = fileSize/oneFileSectionSize + 1
	} else {
		fileSectionCount = fileSize / oneFileSectionSize
	}

	log.Debug("file section count = %v.", fileSectionCount)
	var n int = 0
	for i := int64(0); i < fileSectionCount; i++ {
		_, err = f.Seek(i*oneFileSectionSize, 0)
		if err != nil {
			log.Error("Seek fail!")
			return err
		}

		n, err = f.Read(buf)
		if err != nil {
			log.Error(err.Error())
			return err
		}

		_, err = conn.Write(buf[0:n])
		if err != nil {
			log.Error(err.Error())
			return err
		}
	}

	return nil
}

func downloadFile(filePath string, done chan bool) error {
	defer func() {
		done <- true
	}()

	var err error
	var conn net.Conn
	conn, err = net.Dial("tcp", IPPort)
	if err != nil {
		log.Error("Dial fail! " + err.Error())
		return err
	}

	defer conn.Close()

	buf := make([]byte, 1024)
	//send file name
	copy(buf[0:], []byte(DownloadCommond))
	copy(buf[OperationSecOffset:], []byte(filePath))

	//fmt.Println(buf)

	_, err = conn.Write(buf)
	if err != nil {
		log.Error(err.Error())
		return err
	}

	var n int = 0
	//get file size
	n, err = conn.Read(buf)
	if err != nil {
		log.Error(err.Error())
		return err
	}

	if n < 8 {
		log.Error("Can not get file size!")
		return err
	}

	var fullPath string
	if runtime.GOOS == "windows" {
		fullPath = DownloadDir + strings.Replace(filePath, "/", "\\", -1)
	} else {
		fullPath = DownloadDir + strings.Replace(filePath, "\\", "/", -1)
	}

	dir := GetDir(fullPath)

	_, err = os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			err := os.MkdirAll(dir, os.ModePerm)
			if err != nil {
				log.Error(err.Error())
				return err
			}
		}
	}

	var f *os.File
	f, err = os.Create(fullPath)
	if err != nil {
		log.Error(err.Error())
		return err
	}

	defer f.Close()

	fileSize := BytesToInt64(buf[0:8])

	if fileSize <= 1024 {
		buf = make([]byte, fileSize)
	} else {
		buf = make([]byte, 1024)
	}

	log.Debug("file size = %v", fileSize)
	var offset int64 = 0
	for {
		n, err = conn.Read(buf)
		if err != nil {
			log.Error(err.Error())
			return err
		}

		//fmt.Println("data =", buf)
		//fmt.Println("offset =", offset, ", n =", n)
		_, err = f.Seek(offset, 0)
		if err != nil {
			log.Error("Seek fail!")
			return err
		}

		_, err = f.Write(buf[0:n])
		if err != nil {
			log.Error(err.Error())
			return err
		}

		offset += int64(n)

		if offset >= fileSize {
			break
		}
	}

	return nil
}

func showAllFiles() error {
	conn, err := net.Dial("tcp", IPPort)
	if err != nil {
		log.Error("Dial fail! " + err.Error())
		return err
	}

	defer conn.Close()

	buf := make([]byte, 1024)

	//file size and name 16 + 256 + 8
	copy(buf[0:], []byte(ShowCommond))

	//fmt.Println(buf)

	_, err = conn.Write(buf)
	if err != nil {
		log.Error(err.Error())
		return err
	}

	var n int = 0
	//get file size
	n, err = conn.Read(buf)
	if err != nil {
		log.Error(err.Error())
		return err
	}

	fmt.Println("n =", n)
	fmt.Println(string(GetValidByte(buf)))

	return nil
}

func searchFiles(fileNames []string) error {
	conn, err := net.Dial("tcp", IPPort)
	if err != nil {
		log.Error("Dial fail! " + err.Error())
		return err
	}

	defer conn.Close()

	buf := make([]byte, 1024)

	//file size and name 16 + 256 + 8
	copy(buf[0:], []byte(SearchCommond))

	var offset int = OperationSecOffset

	for _, fileName := range fileNames {
		copy(buf[offset:], []byte(fileName))
		offset += len(fileName)
		if offset >= 1024 {
			break
		}

		copy(buf[offset:], []byte("\n"))

		offset += len("\n")
		if offset >= 1024 {
			break
		}
	}

	_, err = conn.Write(buf)
	if err != nil {
		log.Error(err.Error())
		return err
	}

	//get json
	var n int = 0
	n, err = conn.Read(buf)
	if err != nil {
		log.Error(err.Error())
		return err
	}

	fileJson := buf[0:n]

	//fmt.Println("n =", n)
	//fmt.Println(string(fileJson))

	fileMap := make(map[string]string)
	json.Unmarshal(fileJson, &fileMap)

	for file := range fileMap {
		fmt.Println(file, ":", fileMap[file])
	}

	return nil
}

func ftpClientInit() {
	if runtime.GOOS == "windows" {
		DownloadDir = "D:\\LearnPro\\Golang\\download\\"
		IPPort = "192.168.195.129:1263"
	} else {
		DownloadDir = "/home/thomas/download/"
		IPPort = "127.0.0.1:1263"
	}
}

func main() {
	ftpClientInit()
	args := os.Args[1:]
	if len(args) == 0 {
		log.Error("Need Args!")
		return
	}

	op := args[0]

	//log.Debug("op = %v.", op)

	var done chan bool
	var filePaths []string
	switch op {
	case UploadCommond:
		if len(args) < 2 {
			log.Error("Need file path!")
			return
		}
		filePaths = args[1:]
		log.Debug("file path = %v", filePaths)
		if len(filePaths) > 0 {
			done = make(chan bool, len(filePaths))
		}

		for _, filePath := range filePaths {
			go uploadFile(filePath, done)
		}

		for i := 0; i < len(filePaths); i++ {
			<-done
		}
	case DownloadCommond:
		if len(args) < 2 {
			log.Error("Need file path!")
			return
		}
		filePaths = args[1:]
		log.Debug("file path = %v", filePaths)
		if len(filePaths) > 0 {
			done = make(chan bool, len(filePaths))
		}

		for _, filePath := range filePaths {
			go downloadFile(filePath, done)
		}

		for i := 0; i < len(filePaths); i++ {
			<-done
		}
	case ShowCommond:
		showAllFiles()
	case SearchCommond:
		if len(args) < 2 {
			log.Error("Need file name!")
			return
		}
		fileNames := args[1:]
		searchFiles(fileNames)
	default:
		log.Error("Unknown commond!")
	}
}
