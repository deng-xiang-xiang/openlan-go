package vswitch

import (
    "os"
    "bufio"
    "log"
    "strings"
    "fmt"
    "errors"
    "sort"
    "net"
    "sync"
    "time"
    "encoding/json"

    "github.com/songgao/water"
    "github.com/milosgajdos83/tenus"
    "github.com/danieldin95/openlan-go/libol"
)

type Point struct {
    Client *libol.TcpClient
    Device *water.Interface
}

func NewPoint(c *libol.TcpClient, d *water.Interface) (this *Point) {
    this = &Point {
        Client: c,
        Device: d,
    }

    return
}

type VSwitchWroker struct {
    //Public variable
    Server *TcpServer
    Neighbor *Neighborer

    //Private variable
    verbose int
    br tenus.Bridger
    keys []int
    hooks map[int]func(*libol.TcpClient, *libol.Frame) error
    ifmtu int

    clientsLock sync.RWMutex
    clients map[*libol.TcpClient]*Point
    usersLock sync.RWMutex
    users map[string]*User
    newtime int64
}

func NewVSwitchWroker(server *TcpServer, c *Config) (this *VSwitchWroker) {
    this = &VSwitchWroker {
        Server: server,
        Neighbor: NewNeighborer(c),

        verbose: c.Verbose,
        br: nil,
        ifmtu: 1514,
        hooks: make(map[int]func(*libol.TcpClient, *libol.Frame) error),
        keys: make([]int, 0, 1024),
        clients: make(map[*libol.TcpClient]*Point, 1024),
        users: make(map[string]*User, 1024),
        newtime: time.Now().Unix(),
    }

    this.newBr(c.Brname, c.Ifaddr)
    this.register()
    this.loadUsers(c.Password)

    return 
}

func (this *VSwitchWroker) register() {
    this.setHook(0x10, this.Neighbor.OnFrame)
    this.setHook(0x00, this.checkAuth)
    this.setHook(0x01, this.handleReq)

    this.showHook()
}

func (this *VSwitchWroker) loadUsers(path string) error {
    file, err := os.Open(path)
    if err != nil {
        return err
    }

    defer file.Close()
    reader := bufio.NewReader(file)

    for {
        line, err := reader.ReadString('\n')
        if err != nil {
            break
        }
        
        values := strings.Split(line, ":")
        if len(values) == 2 {
            user := &User{Name: values[0], Password: strings.TrimSpace(values[1])}
            this.AddUser(user)
        }
    }

    return nil
}

func (this *VSwitchWroker) newBr(brname string, addr string) {
    addrs := strings.Split(this.Server.GetAddr(), ":")
    if len(addrs) != 2 {
        log.Printf("Error| VSwitchWroker.newBr: address: %s", this.Server.GetAddr())
        return
    }

    var err error
    var br tenus.Bridger

    if (brname == "") {
        brname = fmt.Sprintf("brol-%s", addrs[1])
        br, err = tenus.BridgeFromName(brname)
        if err != nil {
            br, err = tenus.NewBridgeWithName(brname)
            if err != nil {
                log.Printf("Error| VSwitchWroker.newBr: %s", err)
            }
        }
    } else {
        br, err = tenus.BridgeFromName(brname)
        if err != nil {
            log.Printf("Error| VSwitchWroker.newBr: %s", err)
        }
    }

    if err = br.SetLinkUp(); err != nil {
        log.Printf("Error| VSwitchWroker.newBr: %s", err)
    }

    log.Printf("Info| VSwitchWroker.newBr %s", brname)

    ip, ipnet, err := net.ParseCIDR(addr)
    if err != nil {
        log.Printf("Error| VSwitchWroker.newBr.ParseCIDR %s : %s", addr, err)
    }

    if err := br.SetLinkIp(ip, ipnet); err != nil {
        log.Printf("Error| VSwitchWroker.newBr.SetLinkIp %s : %s", brname, err)
    }

    this.br = br
}

func (this *VSwitchWroker) newTap() (*water.Interface, error) {
    log.Printf("Debug| VSwitchWroker.newTap")  
    ifce, err := water.New(water.Config {
        DeviceType: water.TAP,
    })
    if err != nil {
        log.Printf("Error| VSwitchWroker.newTap: %s", err)
        return nil, err
    }
    
    link, err := tenus.NewLinkFrom(ifce.Name())
    if err != nil {
        log.Printf("Error| VSwitchWroker.newTap: Get ifce %s: %s", ifce.Name(), err)
        return nil, err
    }
    
    if err := link.SetLinkUp(); err != nil {
        log.Printf("Error| VSwitchWroker.newTap: ", err)
    }

    if err := this.br.AddSlaveIfc(link.NetInterface()); err != nil {
        log.Printf("Error| VSwitchWroker.newTap: Switch ifce %s: %s", ifce.Name(), err)
        return nil, err
    }

    log.Printf("Info| VSwitchWroker.newTap %s", ifce.Name())  

    return ifce, nil
}

func (this *VSwitchWroker) Start() {
    go this.Server.GoAccept()
    go this.Server.GoLoop(this.onClient, this.onRecv, this.onClose)
}

func (this *VSwitchWroker) showHook() {
    for _, k := range this.keys {
        log.Printf("Debug| VSwitchWroker.showHool k:%d func: %p", k, this.hooks[k])
    }
} 

func (this *VSwitchWroker) setHook(index int, hook func(*libol.TcpClient, *libol.Frame) error) {
    this.hooks[index] = hook
    this.keys = append(this.keys, index)
    sort.Ints(this.keys)
}

func (this *VSwitchWroker) onHook(client *libol.TcpClient, data []byte) error {
    frame := libol.NewFrame(data)

    for _, k := range this.keys {
        if this.IsVerbose() {
            log.Printf("Debug| VSwitchWroker.onHook k:%d", k)
        }
        if f, ok := this.hooks[k]; ok {
            if err := f(client, frame); err != nil {
                return err
            }
        }   
    }

    return nil
}

func (this *VSwitchWroker) checkAuth(client *libol.TcpClient, frame *libol.Frame) error {
    if this.IsVerbose() {
        log.Printf("Debug| VSwitchWroker.checkAuth % x.", frame.Data)
    }

    if libol.IsInst(frame.Data) {
        action := libol.DecAction(frame.Data)
        log.Printf("Debug| VSwitchWroker.checkAuth.action: %s", action)

        if action == "logi=" {
            if err := this.handlelogin(client, libol.DecBody(frame.Data)); err != nil {
                log.Printf("Error| VSwitchWroker.checkAuth: %s", err)
                client.SendResp("login", err.Error())
                client.Close()
                return err
            }
            client.SendResp("login", "okay.")
        }

        return nil
    }

    if client.Status != libol.CL_AUTHED {
        client.Droped++
        if this.IsVerbose() {
            log.Printf("Debug| VSwitchWroker.onRecv: %s unauth", client.GetAddr())
        }
        return errors.New("Unauthed client.")
    }

    return nil
}

func  (this *VSwitchWroker) handlelogin(client *libol.TcpClient, data string) error {
    if this.IsVerbose() {
        log.Printf("Debug| VSwitchWroker.handlelogin: %s", data)
    }
    user := &User {}
    if err := json.Unmarshal([]byte(data), user); err != nil {
        return errors.New("Invalid json data.")
    }

    name := user.Name
    if user.Token != "" {
        name = user.Token
    }
    _user := this.GetUser(name)
    if _user != nil {
        if _user.Password == user.Password {
            client.Status = libol.CL_AUTHED
            log.Printf("Info| VSwitchWroker.handlelogin: %s Authed", client.GetAddr())
            this.onAuth(client)
            return nil
        }

        client.Status = libol.CL_UNAUTH
    }

    return errors.New("Auth failed.")
}

func (this *VSwitchWroker) handleReq(client *libol.TcpClient, frame *libol.Frame) error {
    return nil
}

func (this *VSwitchWroker) onClient(client *libol.TcpClient) error {
    client.Status = libol.CL_CONNECTED
    log.Printf("Info| VSwitchWroker.onClient: %s", client.GetAddr()) 

    return nil
}

func (this *VSwitchWroker) onAuth(client *libol.TcpClient) error {
    if client.Status != libol.CL_AUTHED {
        return errors.New("not authed.")
    }

    log.Printf("Info| VSwitchWroker.onAuth: %s", client.GetAddr())   

    ifce, err := this.newTap()
    if err != nil {
        return err
    }

    this.AddPoint(NewPoint(client, ifce))
    
    go this.GoRecv(ifce, client.SendMsg)

    return nil
}

func (this *VSwitchWroker) onRecv(client *libol.TcpClient, data []byte) error {
    //TODO Hook packets such as ARP Learning.
    if this.IsVerbose() {
        log.Printf("Debug| VSwitchWroker.onRecv: %s % x", client.GetAddr(), data)    
    }

    if err := this.onHook(client, data); err != nil {
        if this.IsVerbose() {
            log.Printf("Debug| VSwitchWroker.onRecv: %s dropping by %s", client.GetAddr(), err)
            return err
        }
    }

    point := this.GetPoint(client)
    if point == nil {
        return errors.New("Point not found.")
    }

    ifce := point.Device
    if point == nil || point.Device == nil {
        return errors.New("Tap devices is nil")
    }
 
    if _, err := ifce.Write(data); err != nil {
        log.Printf("Error| VSwitchWroker.onRecv: %s", err)
    }

    return nil
}

func (this *VSwitchWroker) onClose(client *libol.TcpClient) error {
    log.Printf("Info| VSwitchWroker.onClose: %s", client.GetAddr())

    this.DelPoint(client)
    
    return nil
}

func (this *VSwitchWroker) Close() {
    this.Server.Close()
}

func (this *VSwitchWroker) GoRecv(ifce *water.Interface, dorecv func([]byte)(error)) {
    log.Printf("Info| VSwitchWroker.GoRecv: %s", ifce.Name())    
    defer ifce.Close()
    for {
        data := make([]byte, this.ifmtu)
        n, err := ifce.Read(data)
        if err != nil {
            log.Printf("Error| VSwitchWroker.GoRev: %s", err)
            break
        }
        if this.IsVerbose() {
            log.Printf("Debug| VSwitchWroker.GoRev: % x\n", data[:n])
        }

        if err := dorecv(data[:n]); err != nil {
            log.Printf("Error| VSwitchWroker.GoRev: do-recv %s %s", ifce.Name(), err)
        }
    }
}

func (this *VSwitchWroker) IsVerbose() bool {
    return this.verbose != 0
}


func (this *VSwitchWroker) AddUser(user *User) {
    this.usersLock.Lock()
    defer this.usersLock.Unlock()

    name := user.Name 
    if name == "" {
        name = user.Token
    }
    this.users[name] = user
}

func (this *VSwitchWroker) GetUser(name string) *User {
    this.usersLock.RLock()
    defer this.usersLock.RUnlock()

    if u, ok := this.users[name]; ok {
        return u
    }

    return nil
}

func (this *VSwitchWroker) ListUser() chan *User {
    c := make(chan *User, 128)

    go func() {
        this.usersLock.RLock()
        defer this.usersLock.RUnlock()

        for _, u := range this.users {
            c <- u
        }
        c <- nil //Finish channel by nil.
    }()

    return c
}

func (this *VSwitchWroker) AddPoint(p *Point) {
    this.clientsLock.Lock()
    defer this.clientsLock.Unlock()

    this.clients[p.Client] = p
}

func (this *VSwitchWroker) GetPoint(c *libol.TcpClient) *Point {
    this.clientsLock.RLock()
    defer this.clientsLock.RUnlock()

    if p, ok := this.clients[c]; ok {
        return p
    }
    return nil
}

func (this *VSwitchWroker) DelPoint(c *libol.TcpClient) {
    this.clientsLock.Lock()
    defer this.clientsLock.Unlock()
    
    if p, ok := this.clients[c]; ok {
        p.Device.Close()
        delete(this.clients, p.Client)
    }
}

func (this *VSwitchWroker) ListPoint() chan *Point {
    c := make(chan *Point, 128)

    go func() {
        this.clientsLock.RLock()
        defer this.clientsLock.RUnlock()

        for _, p := range this.clients {
            c <- p
        }
        c <- nil //Finish channel by nil.
    }()

    return c
}

func (this *VSwitchWroker) UpTime() int64 {
    return time.Now().Unix() - this.newtime
}