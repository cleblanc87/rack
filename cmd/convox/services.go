package main

import (
	"fmt"

	"github.com/convox/rack/Godeps/_workspace/src/github.com/codegangsta/cli"
	"github.com/convox/rack/cmd/convox/stdcli"
)

type Service struct {
	Name     string
	Password string
	Type     string
	Status   string
	URL      string

	App string

	Stack string

	Outputs    map[string]string
	Parameters map[string]string
	Tags       map[string]string
}

type Services []Service

func init() {
	stdcli.RegisterCommand(cli.Command{
		Name:        "services",
		Description: "manage services",
		Usage:       "",
		Action:      cmdServices,
		Subcommands: []cli.Command{
			{
				Name:        "create",
				Description: "create a new service",
				Usage:       "<type> <name> [--url=value]",
				Action:      cmdServiceCreate,
				Flags: []cli.Flag{
					cli.StringFlag{
						Name:  "url",
						Usage: "URL to 3rd party service, e.g. logs1.papertrailapp.com:11235",
					},
				},
			},
			{
				Name:        "delete",
				Description: "delete a service",
				Usage:       "<name>",
				Action:      cmdServiceDelete,
			},
			{
				Name:        "info",
				Description: "info about a service",
				Usage:       "<name>",
				Action:      cmdServiceInfo,
			},
			{
				Name:        "link",
				Description: "create a link between a service and an app",
				Usage:       "<name>",
				Action:      cmdLinkCreate,
				Flags:       []cli.Flag{appFlag},
			},
			{
				Name:        "unlink",
				Description: "Delete a link between a service and an app",
				Usage:       "<name>",
				Action:      cmdLinkDelete,
				Flags:       []cli.Flag{appFlag},
			},
		},
	})
}

func cmdServices(c *cli.Context) {
	services, err := rackClient(c).GetServices()

	if err != nil {
		stdcli.Error(err)
		return
	}

	t := stdcli.NewTable("NAME", "TYPE", "STATUS")

	for _, service := range services {
		t.AddRow(service.Name, service.Type, service.Status)
	}

	t.Print()
}

func cmdServiceCreate(c *cli.Context) {
	if len(c.Args()) != 2 {
		stdcli.Usage(c, "create")
		return
	}

	t := c.Args()[0]
	name := c.Args()[1]
	url := c.String("url")

	fmt.Printf("Creating %s (%s)... ", name, t)

	_, err := rackClient(c).CreateService(t, name, url)

	if err != nil {
		stdcli.Error(err)
		return
	}

	fmt.Println("CREATING")
}

func cmdServiceDelete(c *cli.Context) {
	if len(c.Args()) != 1 {
		stdcli.Usage(c, "delete")
		return
	}

	name := c.Args()[0]

	fmt.Printf("Deleting %s... ", name)

	_, err := rackClient(c).DeleteService(name)

	if err != nil {
		stdcli.Error(err)
		return
	}

	fmt.Println("DELETING")
}

func cmdServiceInfo(c *cli.Context) {
	if len(c.Args()) != 1 {
		stdcli.Usage(c, "info")
		return
	}

	name := c.Args()[0]

	service, err := rackClient(c).GetService(name)

	if err != nil {
		stdcli.Error(err)
		return
	}

	fmt.Printf("Name    %s\n", service.Name)
	fmt.Printf("Status  %s\n", service.Status)
	fmt.Printf("URL     %s\n", service.URL)
}

func cmdLinkCreate(c *cli.Context) {
	_, app, err := stdcli.DirApp(c, ".")

	if err != nil {
		stdcli.Error(err)
		return
	}

	if len(c.Args()) != 1 {
		stdcli.Usage(c, "link")
		return
	}

	name := c.Args()[0]

	_, err = rackClient(c).CreateLink(app, name)

	if err != nil {
		stdcli.Error(err)
		return
	}

	fmt.Printf("Linked %s to %s\n", name, app)
}

func cmdLinkDelete(c *cli.Context) {
	_, app, err := stdcli.DirApp(c, ".")

	if err != nil {
		stdcli.Error(err)
		return
	}

	if len(c.Args()) != 1 {
		stdcli.Usage(c, "unlink")
		return
	}

	name := c.Args()[0]

	_, err = rackClient(c).DeleteLink(app, name)

	if err != nil {
		stdcli.Error(err)
		return
	}

	fmt.Printf("Unlinked %s from %s\n", name, app)
}
