package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Songmu/prompter"
	"github.com/fatih/color"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/motemen/blogsync/atom"
	wsse "github.com/motemen/go-wsse"
	"github.com/skratchdot/open-golang/open"
	"github.com/urfave/cli"
	yaml "gopkg.in/yaml.v2"
)

const (
	// BLOGSYNC is https://github.com/motemen/blogsync
	BLOGSYNC = "blogsync"
	// ExitOK is 0
	ExitOK = 0
	// ExitError is 1
	ExitError = 1
)

type blogs []*blog
type blog struct {
	Path    string
	Title   string
	Date    time.Time
	URL     string
	EditURL string
	Status  string
}

func (b *blog) displayList() string {
	return fmt.Sprintf("%s | %s | %s", b.Status, b.Date.Format("2006-01-02 15:04:05"), b.Title)
}

func (b *blog) isDraft() bool {
	return b.Status == "draft "
}

type config struct {
	userInfo struct {
		blogID   string
		username string
		password string
	}
	defaultset struct {
		localroot string
		entryroot string
		draftroot string
	}
	selector struct {
		cmd    string
		option string
	}
}

type ymlvalues struct {
	Username  string `yaml:"username"`
	Password  string `yaml:"password"`
	LocalRoot string `yaml:"local_root"`
	DraftRoot string `yaml:"draft_root"`
	Cmd       string `yaml:"cmd"`
	Option    string `yaml:"option"`
}

func editor() string {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}
	return editor
}

func (cfg *config) configPath() (string, error) {
	pwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	confPath := filepath.Join(pwd, "blogsync.yaml")
	if !fileExists(confPath) {
		home, err := homedir.Dir()
		if err != nil {
			return "", err
		}
		confPath = filepath.Join(home, ".config", "blogsync", "config.yaml")
		if !fileExists(confPath) {
			return "", fmt.Errorf("Error: config file is not exists.\n" +
				"See also: https://github.com/humangas/hblog#configuration\n")
		}
	}

	return confPath, nil
}

func genPostedBlog(cfg *config, path string) (*blog, error) {
	b := &blog{Path: path}

	// isPosted
	if strings.Index(path, cfg.defaultset.entryroot) == 0 {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		var headerCnt int
		for scanner.Scan() {
			if strings.HasPrefix(scanner.Text(), "---") {
				// --- の2回目で終了する
				if headerCnt == 1 {
					break
				}
				headerCnt++
			}
			if strings.HasPrefix(scanner.Text(), "Title:") {
				str := strings.Replace(scanner.Text(), "Title: ", "", 1)
				b.Title = str
				continue
			}
			if strings.HasPrefix(scanner.Text(), "Date:") {
				// Ref: Date to String:
				// https://play.golang.org/p/pKRHl7AuFJG
				str := strings.Replace(scanner.Text(), "Date: ", "", 1)
				b.Date, _ = time.Parse(time.RFC3339, str)
				continue
			}
			if strings.HasPrefix(scanner.Text(), "URL:") {
				str := strings.Replace(scanner.Text(), "URL: ", "", 1)
				b.URL = str
				continue
			}
			if strings.HasPrefix(scanner.Text(), "EditURL:") {
				str := strings.Replace(scanner.Text(), "EditURL: ", "", 1)
				b.EditURL = str
				continue
			}
			b.Status = "posted"
		}
	} else {
		f, err := os.Stat(path)
		if err != nil {
			return nil, err
		}

		b.Date = f.ModTime()
		_, b.Title = filepath.Split(fileWithoutExt(path))
		b.Status = "draft "
	}

	return b, nil
}

// TODO 現状一つのブログサイトにしか対応してない。一つしか運用しないので更新する予定はない。
func (cfg *config) load() error {
	path, err := cfg.configPath()
	if err != nil {
		return err
	}
	buf, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	var ymv map[string]ymlvalues
	if err := yaml.Unmarshal(buf, &ymv); err != nil {
		return err
	}

	cfg.defaultset.localroot, err = homedir.Expand(ymv["default"].LocalRoot)
	if err != nil {
		return err
	}
	cfg.defaultset.draftroot, err = homedir.Expand(ymv["default"].DraftRoot)
	if err != nil {
		return err
	}
	delete(ymv, "default")

	cfg.selector.cmd = ymv["selector"].Cmd
	cfg.selector.option = ymv["selector"].Option
	delete(ymv, "selector")

	// The loop is only once. Because it deleted in the above.
	for key, v := range ymv {
		cfg.userInfo.blogID = key
		cfg.userInfo.username = v.Username
		cfg.userInfo.password = v.Password
	}

	cfg.defaultset.entryroot = filepath.Join(cfg.defaultset.localroot, cfg.userInfo.blogID, "entry")

	return nil
}

func (cfg *config) check() error {
	var errmsg []string
	if cfg.userInfo.blogID == "" {
		errmsg = append(errmsg, "- Not found: blogID")
	}
	if cfg.userInfo.username == "" {
		errmsg = append(errmsg, "- Not found: blogID > username")
	}
	if cfg.userInfo.password == "" {
		errmsg = append(errmsg, "- Not found: blogID > password")
	}
	if cfg.defaultset.localroot == "" {
		errmsg = append(errmsg, "- Not found: default > local_root")
	}
	if cfg.defaultset.draftroot == "" {
		errmsg = append(errmsg, "- Not found: default > draft_root")
	}
	if cfg.selector.cmd == "" {
		errmsg = append(errmsg, "- Not found: selector > cmd")
	}
	if cfg.selector.option == "" {
		errmsg = append(errmsg, "- Not found: selector > option")
	}

	if len(errmsg) > 0 {
		return fmt.Errorf("\nconfig error:\n%s", strings.Join(errmsg, "\n"))
	}
	return nil
}

func main() {
	app := cli.NewApp()
	app.Name = "hblog"
	app.Usage = "CLI client for Hatena blog"
	app.UsageText = "hblog [global options] command [<arguments>]"
	// app.Description = "CLI client for Hatena blog"
	app.Version = fmt.Sprintf("%s", VERSION)
	app.Commands = []cli.Command{
		commandList,
		commandNew,
		commandEdit,
		commandPull,
		commandPush,
		commandConfig,
		commandBrowse,
	}

	os.Exit(returnCode(app.Run(os.Args)))
}

var commandConfig = cli.Command{
	Name:    "config",
	Aliases: []string{"c"},
	Usage:   "Edit config file",
	Action:  cmdConfig,
}

var commandList = cli.Command{
	Name:    "list",
	Aliases: []string{"l"},
	Usage:   "List entries",
	Action:  cmdList,
}

var commandPull = cli.Command{
	Name:    "pull",
	Aliases: []string{"p"},
	Usage:   "Pull entries from remote",
	Action:  cmdPull,
}

var commandEdit = cli.Command{
	Name:    "edit",
	Aliases: []string{"e"},
	Usage:   "Edit entries",
	Action:  cmdEdit,
}

var commandNew = cli.Command{
	Name:    "new",
	Aliases: []string{"n"},
	Usage:   "New entries in draft",
	Description: "Under the draft directly, create a file with the name <title>.md\n   " +
		"You can post this file with the push command.\n   " +
		"draft directory is \"config.yaml > default > local_root + draft\"",
	ArgsUsage: "<title>",
	Action:    cmdNew,
}

var commandPush = cli.Command{
	Name:      "push",
	Usage:     "Push local entries to remote",
	ArgsUsage: "[path]",
	Action:    cmdPush,
}

var commandBrowse = cli.Command{
	Name:    "browse",
	Aliases: []string{"b"},
	Usage:   "Open entries web site with browser",
	Action:  cmdBrowse,
}

func cmdConfig(c *cli.Context) error {
	var cfg config
	if err := cfg.load(); err != nil {
		return err
	}

	var cmd *exec.Cmd
	path, err := cfg.configPath()
	if err != nil {
		return err
	}
	cmd = exec.Command(editor(), path)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin

	return cmd.Run()
}

func cmdList(c *cli.Context) error {
	var cfg config
	if err := cfg.load(); err != nil {
		return err
	}
	if err := cfg.check(); err != nil {
		return err
	}

	bs, err := bloglist(&cfg)
	if err != nil {
		return err
	}
	if bs == nil {
		return fmt.Errorf("Can not find files. " +
			"Please do \"pull\" or \"new\" command in advance.")
	}

	sort.Slice(bs, func(i, j int) bool {
		return bs[i].Date.Format("2006-01-02 15:04:05") < bs[j].Date.Format("2006-01-02 15:04:05")
	})

	for _, v := range bs {
		fmt.Println(v.displayList())
	}

	return nil
}

func cmdPull(c *cli.Context) error {
	var cfg config
	if err := cfg.load(); err != nil {
		return err
	}
	if err := cfg.check(); err != nil {
		return err
	}

	// TODO: 本体がhomedir.Expand()してないのでパスはフルパスにしている
	if err := runBlogsync("pull", os.Stdin, os.Stdout, cfg.userInfo.blogID); err != nil {
		return err
	}

	bs, err := bloglist(&cfg)
	if err != nil {
		return err
	}
	if bs == nil {
		return fmt.Errorf("Can not find files")
	}

	links, err := entriesLink(&cfg)
	if err != nil {
		return err
	}

	var delfiles []string
LABEL:
	for _, b := range bs {
		for i, l := range links {
			if b.URL == l {
				links = append(links[:i], links[i+1:]...)
				continue LABEL
			}
		}
		if !b.isDraft() {
			delfiles = append(delfiles, b.Path)
		}
	}

	for _, v := range delfiles {
		if err := os.Remove(v); err != nil {
			return err
		}
		fmt.Printf("Delete file: %s\n", v)
	}

	return nil
}

func cmdPush(c *cli.Context) error {
	var cfg config
	if err := cfg.load(); err != nil {
		return err
	}
	if err := cfg.check(); err != nil {
		return err
	}

	var b *blog
	var err error
	if c.Args().Present() {
		filePath := c.Args().First()
		if !fileExists(filePath) {
			return fmt.Errorf("Error: %s is not exists", filePath)
		}
		b, err = genPostedBlog(&cfg, filePath)
		if err != nil {
			return err
		}

	} else {
		bs, err := bloglist(&cfg)
		if err != nil {
			return err
		}
		if bs == nil {
			return fmt.Errorf("Can not find files. " +
				"Please do \"pull\" or \"new\" command in advance.")
		}

		b, err = selectFilePath(bs)
		if err != nil {
			return err
		}
		// If not selected, it ends normally
		if b == nil {
			return nil
		}
	}

	if !prompter.YesNo(color.RedString(fmt.Sprintf("Push? %s", b.Title)), false) {
		return nil
	}

	if b.isDraft() {
		f, err := os.Open(b.Path)
		if err != nil {
			return err
		}
		defer f.Close()
		err = runBlogsync("post", f, os.Stdout, "--title", b.Title, cfg.userInfo.blogID, b.Path)
		if prompter.YesNo(color.RedString(fmt.Sprintf("Delete? %s", b.Path)), false) {
			if err := os.Remove(b.Path); err != nil {
				return err
			}
		}
	} else {
		err = runBlogsync("push", os.Stdin, os.Stdout, b.Path)
	}

	return err
}

func cmdEdit(c *cli.Context) error {
	var cfg config
	if err := cfg.load(); err != nil {
		return err
	}
	if err := cfg.check(); err != nil {
		return err
	}

	bs, err := bloglist(&cfg)
	if err != nil {
		return err
	}
	if bs == nil {
		return fmt.Errorf("Can not find files. " +
			"Please do \"pull\" or \"new\" command in advance.")
	}

	blog, err := selectFilePath(bs)
	if err != nil {
		return err
	}
	// If not selected, it ends normally
	if blog == nil {
		return nil
	}

	cmd := exec.Command(editor(), blog.Path)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin

	return cmd.Run()
}

func cmdNew(c *cli.Context) error {
	var cfg config
	if err := cfg.load(); err != nil {
		return err
	}
	if err := cfg.check(); err != nil {
		return err
	}

	if !c.Args().Present() {
		cli.ShowCommandHelp(c, "new")
		return nil
	}

	os.MkdirAll(cfg.defaultset.draftroot, 0700)
	title := c.Args().First()
	filePath := filepath.Join(cfg.defaultset.draftroot, title+".md")

	var cmd *exec.Cmd
	cmd = exec.Command(editor(), filePath)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin

	return cmd.Run()
}

func cmdBrowse(c *cli.Context) error {
	var err error
	var cfg config
	if err = cfg.load(); err != nil {
		return err
	}
	if err := cfg.check(); err != nil {
		return err
	}

	bs, err := bloglist(&cfg)
	if err != nil {
		return err
	}
	if bs == nil {
		return fmt.Errorf("Can not find files. " +
			"Please do \"pull\" command in advance.")
	}

	blog, err := selectFilePath(bs)
	if err != nil {
		return err
	}
	// If not selected, it ends normally
	if blog == nil {
		return nil
	}

	if blog.isDraft() {
		err = open.Run(cfg.defaultset.draftroot)
	} else {
		err = open.Run(blog.URL)
	}

	return err
}

func selectFilePath(blogss ...blogs) (*blog, error) {
	var cfg config
	if err := cfg.load(); err != nil {
		return nil, err
	}

	var bs blogs
	for _, v := range blogss {
		bs = append(bs, v...)
	}

	sort.Slice(bs, func(i, j int) bool {
		return bs[i].Date.Format("2006-01-02 15:04:05") < bs[j].Date.Format("2006-01-02 15:04:05")
	})

	var lines []string
	for _, v := range bs {
		lines = append(lines, v.displayList())
	}

	var buf bytes.Buffer
	var cmd *exec.Cmd
	cmd = exec.Command(cfg.selector.cmd, (strings.Split(cfg.selector.option, " "))[0:]...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = &buf
	cmd.Stdin = strings.NewReader(strings.Join(lines, "\n"))

	if err := cmd.Run(); err != nil {
		// If not selected, it ends normally
		if len(buf.String()) == 0 {
			return nil, nil
		}
		return nil, err
	}

	filename := strings.TrimSpace(buf.String())
	key := strings.TrimSpace(strings.Split(filename, "|")[1])

	var v *blog
	for _, v = range bs {
		// TODO: keyがdatetime、それが2以上ある場合正しく動かないが、かぶる可能性は低いのでこのままにしとく
		if key == v.Date.Format("2006-01-02 15:04:05") {
			// return する v が決定
			break
		}
	}

	return v, nil
}

func runBlogsync(subcommand string, r io.Reader, w io.Writer, args ...string) error {
	var cmd *exec.Cmd
	cmd = exec.Command(BLOGSYNC, append([]string{subcommand}, args...)[0:]...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = w
	cmd.Stdin = r

	return cmd.Run()
}

func dirwalk(dir string) []string {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		panic(err)
	}

	var paths []string
	for _, file := range files {
		if file.IsDir() {
			paths = append(paths, dirwalk(filepath.Join(dir, file.Name()))...)
			continue
		}
		paths = append(paths, filepath.Join(dir, file.Name()))
	}

	return paths
}

func filterMarkdown(files []string) []string {
	var newfiles []string
	for _, file := range files {
		if strings.HasSuffix(file, ".md") {
			newfiles = append(newfiles, file)
		}
	}

	return newfiles
}

func bloglist(cfg *config) (blogs, error) {
	articlePaths := []string{cfg.defaultset.entryroot, cfg.defaultset.draftroot}

	var list blogs
	for _, path := range articlePaths {
		if fileExists(path) {
			paths := dirwalk(path)
			paths = filterMarkdown(paths)
			for _, path := range paths {
				b, err := genPostedBlog(cfg, path)
				if err != nil {
					return nil, err
				}
				list = append(list, b)
			}
		}
	}

	return list, nil
}

func entriesLink(cfg *config) ([]string, error) {
	entryURL := fmt.Sprintf("https://blog.hatena.ne.jp/%s/%s/atom/entry", cfg.userInfo.username, cfg.userInfo.blogID)
	client := &atom.Client{
		Client: &http.Client{
			Transport: &wsse.Transport{
				Username: cfg.userInfo.username,
				Password: cfg.userInfo.password,
			},
		},
	}

	var links []string
	for {
		feed, err := client.GetFeed(entryURL)
		if err != nil {
			return nil, err
		}

		for _, ae := range feed.Entries {
			alternateLink := ae.Links.Find("alternate")
			if alternateLink == nil {
				return nil, fmt.Errorf("Could not find link[rel=alternate]")
			}

			u, err := url.Parse(alternateLink.Href)
			if err != nil {
				return nil, err
			}

			links = append(links, u.String())
		}

		nextLink := feed.Links.Find("next")
		if nextLink == nil {
			break
		}
		entryURL = nextLink.Href
	}

	return links, nil
}

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

func fileWithoutExt(path string) string {
	return filepath.Base(path[:len(path)-len(filepath.Ext(path))])
}

func returnCode(err error) int {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", os.Args[0], err)
		return ExitError
	}
	return ExitOK
}
