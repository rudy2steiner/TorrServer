package main

import (
    "io/ioutil"
    "server/web/api/utils"
    "fmt"
    "os"
    "path/filepath"
    "strings"
)

func main() {
    dir := os.Args[1]
    path, err := filepath.Abs(os.Args[1])
    if err != nil {
        path = dir
    }
    files, err := ioutil.ReadDir(path)
    for _, file := range files {
        filename := filepath.Join(path, file.Name())
        if strings.ToLower(filepath.Ext(file.Name())) == ".torrent" {
            sp, err := utils.ParseLink("file://" + filename)
            if err == nil {
                fmt.Printf("--------------- \nName: %s,Hash:%s  \n", sp.DisplayName, sp.InfoHash.HexString())
                fmt.Printf("Trackers:\n ")
                for _,t := range sp.Trackers[0] {
                    fmt.Println(t)
                }
            }
        }
    }
    fmt.Printf("Parse dir:%s bt files finished\n", dir)
}