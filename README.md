# hblog
[![MIT License](http://img.shields.io/badge/license-MIT-blue.svg?style=flat)](LICENSE)
[![OS macOS](https://img.shields.io/badge/OS-macOS-blue.svg)](OS)  
はてなブログ用のCLIクライアントです。  
内部で [motemen/blogsync](https://github.com/motemen/blogsync) を利用しています。
これに、new, list, edit 機能などを追加しています。


## Usage
```
NAME:
   hblog - CLI client for Hatena blog

USAGE:
   hblog [global options] command [<arguments>]

VERSION:
   0.1.2

COMMANDS:
     list, l    List entries
     new, n     New entries in draft
     edit, e    Edit entries
     pull       Pull entries from remote
     push       Push local entries to remote
     config, c  Edit config file
     browse, b  Open entries web site with browser
     help, h    Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --help, -h     show help
   --version, -v  print the version

```

pull, push は、blogsync の同機能を呼び出しています。



## Installation
```bash
$ go get github.com/motemen/blogsync
$ go get github.com/humangas/hblog
```

## Dependencies
edit, push, browse 機能がセレクターに依存します。事前に導入が必要です。
- See also: [fzf](https://github.com/junegunn/fzf)


## Configuration
blogsync の Configuration を基本的にそのまま利用します。
- See also: [motemen/blogsync#configuration](https://github.com/motemen/blogsync#configuration)

それに以下の定義を追加します。
```yaml
default:
  draft_root: "new" で作成するファイルが格納されるディレクトリ

selector: 
    cmd: fzf
    option: "--multi --cycle --bind=ctrl-u:half-page-up,ctrl-d:half-page-down"
```
- See also: [config.yaml.template](https://github.com/humangas/hblog/blob/master/config.yaml.template)


`$ hblog config` コマンドでコンフィグファイルを修正することができます。

