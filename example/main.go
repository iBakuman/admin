package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
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

func init() {
	cmd := exec.Command("/bin/bash", "-c", "source ./dev_env && env")
	output, err := cmd.Output()
	if err != nil {
		panic(err)
	}
	envLines := strings.Split(string(output), "\n")
	for _, line := range envLines {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			err := os.Setenv(parts[0], parts[1])
			if err != nil {
				return
			}
		}
	}
}
