package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Unknwon/log"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strings"
)

const (
	UploadCommond      = "upload"
	DownloadCommond    = "download"
	ShowCommond        = "show"
	SearchCommond      = "search"
	UploadDir          = "/home/thomas/upload/"
	OperationSecOffset = 16
	FileNameSecOffset  = 256 + OperationSecOffset
	FileSizeSecOffset  = 8 + FileNameSecOffset
)

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

func convertToFileNames(fileNames []byte) []string {
	var fileList []string
	var startIndex int = 0
	for i, v := range fileNames {
		if v != 0 {
			if v == 10 {
				fileList = append(fileList, string(fileNames[startIndex:i]))
				startIndex = i + 1
			}
		} else {
			break
		}
	}

	return fileList
}

func getFilePath(rootDir, file string) string {
	var filePath string
	filepath.Walk(rootDir, func(path string, f os.FileInfo, err error) error {
		if f == nil {
			log.Error(err.Error())
			return err
		}

		if f.IsDir() {
			return nil
		}

		//log.Debug("%v", path)

		if f.Name() == file {
			//log.Debug("Find it!")
			filePath, _ = filepath.Rel(rootDir, path)
			return errors.New("Found it!")
		}

		return nil
	})

	return filePath
}

func BytesToInt64(b []byte) int64 {
	bytesBuffer := bytes.NewBuffer(b)
	var tmp int64
	binary.Read(bytesBuffer, binary.BigEndian, &tmp)
	return int64(tmp)
}

func Int64ToBytes(n int64) []byte {
	tmp := int64(n)
	bytesBuffer := bytes.NewBuffer([]byte{})
	binary.Write(bytesBuffer, binary.BigEndian, tmp)
	return bytesBuffer.Bytes()
}

func GetDir(fullPath string) string {
	lastIndex := strings.LastIndex(fullPath, "/")
	runes := []rune(fullPath)
	l := lastIndex
	if lastIndex > len(runes) {
		l = len(runes)
	}
	return string(runes[0 : l+1])
}

func uploadFile(conn net.Conn, name string, size int64) error {
	fmt.Println("file name =", name, ", size =", size)

	_, err := os.Stat(UploadDir)
	if err != nil {
		if os.IsNotExist(err) {
			err := os.MkdirAll(UploadDir, os.ModePerm)
			if err != nil {
				log.Error(err.Error())
				return err
			}
		}
	}

	var f *os.File
	f, err = os.Create(UploadDir + name)
	if err != nil {
		log.Error(err.Error())
		return err
	}

	defer f.Close()

	var buf []byte
	if size <= 1024 {
		buf = make([]byte, size)
	} else {
		buf = make([]byte, 1024)
	}
	var n int = 0
	var offset int64 = 0
	for {
		n, err = conn.Read(buf)
		if err != nil {
			log.Error(err.Error())
			return err
		}

		//fmt.Println("read data size =", n, ", data =", buf)
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

		if offset >= size {
			break
		}
	}

	return nil
}

func downloadFile(conn net.Conn, name string) error {
	fmt.Println("file name =", name)

	fileInfo, err := os.Stat(UploadDir + name)
	if err != nil {
		log.Error(err.Error())
		return err
	}

	if fileInfo.IsDir() {
		log.Error("This is a directory!")
		return err
	}

	buf := make([]byte, 1024)

	//send file size
	copy(buf[0:], Int64ToBytes(fileInfo.Size()))
	_, err = conn.Write(buf)
	if err != nil {
		log.Error(err.Error())
		return err
	}

	//send file data
	var f *os.File
	f, err = os.Open(UploadDir + name)
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

	log.Debug("file size = %v, file section count = %v.", fileSize, fileSectionCount)
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

		//fmt.Println("data =", buf[0:n])
		//time.Sleep(10 * time.Millisecond)
		_, err = conn.Write(buf[0:n])
		if err != nil {
			log.Error(err.Error())
			return err
		}
	}

	return nil
}

func showFile(conn net.Conn) error {
	buf := make([]byte, 0, 1024)

	//send file list
	files, err := ioutil.ReadDir(UploadDir)
	if err != nil {
		log.Error(err.Error())
		return err
	}

	for _, file := range files {
		buf = append(buf, []byte(file.Name())...)
		buf = append(buf, []byte("\n")...)
	}

	fmt.Println(string(buf))

	_, err = conn.Write(buf)
	if err != nil {
		log.Error(err.Error())
		return err
	}

	return nil
}

func searchFiles(conn net.Conn, fileList []string) error {
	fileMap := make(map[string]string)

	for _, file := range fileList {
		fileMap[file] = getFilePath(UploadDir, file)
	}

	fileJson, err := json.Marshal(fileMap)

	fmt.Println(string(fileJson))

	var n int
	n, err = conn.Write(fileJson)
	if err != nil {
		log.Error(err.Error())
		return err
	}

	fmt.Printf("Send %v size data.", n)

	return nil
}

func handleConn(conn net.Conn) {
	defer conn.Close()
	log.Debug("Net = %v, Addr = %v.", conn.LocalAddr().Network(), conn.LocalAddr().String())
	log.Debug("Remote net = %v, Remote addr = %v.", conn.RemoteAddr().Network(), conn.RemoteAddr().String())

	var n int = 0
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		log.Error(err.Error())
		return
	}

	if n < FileSizeSecOffset {
		log.Error("Read bytes too little!")
		return
	}

	var op string = string(GetValidByte(buf[0:OperationSecOffset]))

	switch op {
	case UploadCommond:
		fmt.Println("Operation is upload.")
		fileName := string(GetValidByte(buf[OperationSecOffset:FileNameSecOffset]))
		fileSize := BytesToInt64(buf[FileNameSecOffset:FileSizeSecOffset])
		err = uploadFile(conn, fileName, fileSize)
		if err != nil {
			log.Error(err.Error())
		}
		return
	case DownloadCommond:
		fmt.Println("Operation is download.")
		filePath := string(GetValidByte(buf[OperationSecOffset:FileNameSecOffset]))
		err = downloadFile(conn, filePath)
		if err != nil {
			log.Error(err.Error())
		}
		return
	case ShowCommond:
		fmt.Println("Operation is show.")
		err = showFile(conn)
		if err != nil {
			log.Error(err.Error())
		}
		return
	case SearchCommond:
		fmt.Println("Operation is search.")
		fileNames := convertToFileNames(buf[OperationSecOffset:])
		err = searchFiles(conn, fileNames)
		if err != nil {
			log.Error(err.Error())
		}
		return
	default:
		fmt.Println("Unknown operation!")
		return
	}
}

func main() {
	ln, err := net.Listen("tcp", ":1263")
	if err != nil {
		log.Error(err.Error())
		return
	}

	defer ln.Close()

	log.Debug("Net = %v, Addr = %v.", ln.Addr().Network(), ln.Addr().String())

	for {
		log.Debug("New connection!")
		conn, err := ln.Accept()
		if err != nil {
			log.Error(err.Error())
			return
		}

		go handleConn(conn)
	}
}
