// Package gui implements the macOS menu bar interface.
package gui

import (
    "fmt"
    "os/exec"
    "sort"
    "strings"
    "syscall"
    "time"

    "github.com/getlantern/systray"
    "github.com/gen2brain/beeep"
    "github.com/user/goports/internal/ports"
)

// portMenuGroup holds the menu items associated with a particular port.
// by keeping a permanent structure we avoid recreating goroutines and menu
// entries on every reopen.
type portMenuGroup struct {
    parent   *systray.MenuItem
    pidItem  *systray.MenuItem
    cmdItem  *systray.MenuItem
    killItem *systray.MenuItem
    openItem *systray.MenuItem
    visible  bool
}

// Run starts the menu bar application. It blocks until the user quits.
func Run() {
    systray.Run(onReady, onExit)
}

func onReady() {
    // configure tray icon and tooltip
    systray.SetIcon(iconData)
    systray.SetTitle("") // no title, just an icon
    systray.SetTooltip("Ports")

    // static items at the bottom of the menu
    aboutItem := systray.AddMenuItem("About Ports", "Open project page")
    systray.AddSeparator()
    quitItem := systray.AddMenuItem("Quit", "Quit Ports")

    // click handlers for static items
    go func() {
        for range aboutItem.ClickedCh {
            exec.Command("open", "https://github.com/ronreiter/ports").Run()
        }
    }()
    go func() {
        for range quitItem.ClickedCh {
            systray.Quit()
        }
    }()

    // start the ticker loop; it will mutate the menu and therefore must be
    // launched from the same goroutine as onReady. We intentionally start it
    // as a goroutine so that onReady may return immediately.
    go tickerLoop()
}

func onExit() {
    // nothing special to clean up
}

func tickerLoop() {
    portMenu := make(map[int]*portMenuGroup)
    firstRun := true
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()

    for {
        newPorts := ports.AppsByPort()

        var portList []int
        for p := range newPorts {
            portList = append(portList, p)
        }
        sort.Ints(portList)

        // process current ports
        for _, p := range portList {
            entries := newPorts[p]
            if len(entries) == 0 {
                fmt.Printf("warning: no entries for port %d, skipping\n", p)
                continue
            }

            // aggregate metadata across all entries
            var pidStrs []string
            var cmdStrs []string
            for _, e := range entries {
                pidStrs = append(pidStrs, fmt.Sprintf("%d", e.Pid))
                cmdStrs = append(cmdStrs, e.Cmdline)
            }
            title := fmt.Sprintf("%d", p)
            if entries[0].Host != "" {
                title += fmt.Sprintf(" (%s)", entries[0].Host)
            }
            title += fmt.Sprintf(" - %s", entries[0].Name)
            if entries[0].AppBundle != "" {
                title += fmt.Sprintf(" (%s)", entries[0].AppBundle)
            }

            group, exists := portMenu[p]
            if !exists {
                parent := systray.AddMenuItem(title, "")
                pidItem := parent.AddSubMenuItem("PIDs: "+strings.Join(pidStrs, ", "), "")
                cmdItem := parent.AddSubMenuItem("Cmd: "+strings.Join(cmdStrs, " | "), "")
                killItem := parent.AddSubMenuItem("Kill All", "Terminate all processes on this port")
                openItem := parent.AddSubMenuItem(fmt.Sprintf("Open http://localhost:%d", p), "")

                group = &portMenuGroup{
                    parent:   parent,
                    pidItem:  pidItem,
                    cmdItem:  cmdItem,
                    killItem: killItem,
                    openItem: openItem,
                    visible:  true,
                }
                portMenu[p] = group

                if !firstRun {
                    beeep.Notify("Open Port Discovered", fmt.Sprintf("Port %d was just opened by %s", p, entries[0].Name), "")
                }

                // kill handler; runs once per logical port
                go func(port int, kill *systray.MenuItem) {
                    for range kill.ClickedCh {
                        cur := ports.AppsByPort()
                        if ents, ok := cur[port]; ok {
                            for _, e := range ents {
                                syscall.Kill(int(e.Pid), syscall.SIGKILL)
                            }
                            beeep.Notify("Killed Process", fmt.Sprintf("Terminated processes on port %d", port), "")
                        }
                    }
                }(p, killItem)

                // open handler
                go func(port int, open *systray.MenuItem) {
                    for range open.ClickedCh {
                        exec.Command("open", fmt.Sprintf("http://localhost:%d", port)).Run()
                    }
                }(p, openItem)
            } else {
                // update existing group and make visible if it was hidden
                group.parent.SetTitle(title)
                group.pidItem.SetTitle("PIDs: "+strings.Join(pidStrs, ", "))
                group.cmdItem.SetTitle("Cmd: "+strings.Join(cmdStrs, " | "))
                if !group.visible {
                    group.parent.Show()
                    group.pidItem.Show()
                    group.cmdItem.Show()
                    group.killItem.Show()
                    group.openItem.Show()
                    group.visible = true
                    if !firstRun {
                        beeep.Notify("Open Port Discovered", fmt.Sprintf("Port %d was just opened by %s", p, entries[0].Name), "")
                    }
                }
            }
        }

        // hide ports that have closed
        for p, group := range portMenu {
            if _, still := newPorts[p]; !still && group.visible {
                group.parent.Hide()
                group.pidItem.Hide()
                group.cmdItem.Hide()
                group.killItem.Hide()
                group.openItem.Hide()
                group.visible = false
                beeep.Notify("Closed Port Discovered", fmt.Sprintf("Port %d was just closed", p), "")
            }
        }

        firstRun = false
        <-ticker.C
    }
}
