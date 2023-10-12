package main

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/qor5/admin/example/admin"
)

func main() {
	h := admin.Router()

	port := os.Getenv("PORT")
	if len(port) == 0 {
		port = "9000"
	}
	fmt.Println("Served at http://localhost:" + port)

	mux := http.NewServeMux()
	mux.Handle("/",
		middleware.RequestID(
			middleware.Logger(
				middleware.Recoverer(h),
			),
		),
	)
	err := http.ListenAndServe("localhost:"+port, mux)
	if err != nil {
		panic(err)
	}
}

func ConfigureEnv() {
	file, err := os.Open("./dev_env")
	if err != nil {
		panic(err)
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			panic(err)
		}
	}(file)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "export") || strings.HasPrefix(line, "EXPORT") {
			parts := strings.SplitN(line, "=", 2)
			key := strings.TrimPrefix(parts[0], "export ")
			val := strings.TrimSpace(parts[1])
			if v, has := os.LookupEnv(key); !has {
				val := strings.Trim(val, `"`)
				fmt.Printf("set environment variable %s to %s.\n", key, val)
				err := os.Setenv(key, val)
				if err != nil {
					panic(err)
				}
			} else {
				fmt.Printf("environment variable %s has already exists, its value is %s.\n", key, v)
			}
		}
	}
}

func init() {
	ConfigureEnv()
}
