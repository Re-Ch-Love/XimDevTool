package main

import (
	"XimDevTool/commands/preview"
	"fmt"
	"github.com/urfave/cli/v2"
	"os"
)

var app *cli.App

func init() {
	app = cli.NewApp()
	app.Name = "XimDevTool"
	app.Authors = []*cli.Author{
		{
			Name: "Re-Ch",
		},
	}
	app.Version = "0.0.1"
	app.ExitErrHandler = func(ctx *cli.Context, err error) {
		if err != nil {
			fmt.Println("Fatal Error:", err)
			os.Exit(1)
		}
	}
	app.Commands = []*cli.Command{
		{
			Name:  "preview",
			Usage: "preview a component",
			Subcommands: []*cli.Command{
				{
					Name:  "component",
					Usage: "preview a component",
					Flags: []cli.Flag{
						&cli.StringFlag{
							Name:     "address",
							Aliases:  []string{"addr"},
							Value:    "localhost:8080",
							Usage:    "address to listen on",
							Required: true,
						},
						&cli.PathFlag{
							Name:     "path",
							Value:    "",
							Usage:    "component path",
							Required: true,
						},
						&cli.StringFlag{
							Name:     "varName",
							Value:    "",
							Usage:    "component variable name",
							Required: true,
						},
					},
					Action: func(context *cli.Context) error {
						return preview.Component(context.String("addr"), context.Path("path"), context.String("varName"))
					},
				},
				{
					Name:  "project",
					Usage: "preview a project",
					Flags: []cli.Flag{
						&cli.StringFlag{
							Name:     "address",
							Aliases:  []string{"addr"},
							Value:    "localhost:8080",
							Usage:    "address to listen on",
							Required: true,
						},
						&cli.PathFlag{
							Name:     "path",
							Value:    "",
							Usage:    "component path",
							Required: true,
						},
					},
					Action: func(context *cli.Context) error {
						return preview.Project(context.String("addr"), context.Path("path"))
					},
				},
			},
		},
	}
}

func main() {
	err := app.Run(os.Args)
	if err != nil {
		fmt.Println("Fatal Error:", err)
		return
	}
}
