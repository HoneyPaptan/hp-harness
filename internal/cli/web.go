package cli

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/HoneyPaptan/hp-harness/internal/web"
)

const defaultWebPort = 7878

type webOpts struct {
	run    runOpts
	port   int
	listen string
}

func cmdWeb(ctx context.Context, env Env, args []string) int {
	o, err := parseWebFlags(env, args)
	if err != nil {
		return exitUsage(env, err)
	}
	if err := checkWebStart(env, o.run); err != nil {
		return exitUsage(env, err)
	}
	dir, err := resolveWorkDir(env, o.run.dir)
	if err != nil {
		return exitErr(env, "", "", err)
	}
	ln, err := webListener(env, o)
	if err != nil {
		return exitErr(env, "", "", err)
	}
	return serveWeb(ctx, env, ln, o, dir)
}

func parseWebFlags(env Env, args []string) (webOpts, error) {
	fs, o, fb := runFlagSet(env, "web")
	port := fs.Int("port", defaultWebPort, "port on 127.0.0.1")
	listen := fs.String("listen", "", "host:port to bind")
	if err := fs.Parse(args); err != nil {
		return webOpts{}, err
	}
	if len(fs.Args()) != 0 {
		return webOpts{}, usagef("web takes no prompt")
	}
	if o.detach {
		return webOpts{}, usagef("web cannot detach")
	}
	if *listen != "" && *port != defaultWebPort {
		return webOpts{}, usagef("use --port or --listen, not both")
	}
	o.fallbacks = append([]string(nil), *fb...)
	return webOpts{run: *o, port: *port, listen: *listen}, nil
}

func checkWebStart(env Env, o runOpts) error {
	if o.model != "" || env.Getenv("HP_MODEL") != "" {
		if _, _, err := resolveRunSpecs(o, env.Getenv); err != nil {
			return err
		}
	}
	_, err := openJev(env, o.jev)
	return err
}

func webListener(env Env, o webOpts) (net.Listener, error) {
	if o.listen != "" {
		return net.Listen("tcp", o.listen)
	}
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(o.port))
	ln, err := net.Listen("tcp", addr)
	if err == nil {
		return ln, nil
	}
	ln, freeErr := net.Listen("tcp", "127.0.0.1:0")
	if freeErr != nil {
		return nil, err
	}
	fmt.Fprintf(env.Stderr, "port %d is taken, using %d\n", o.port, ln.Addr().(*net.TCPAddr).Port)
	return ln, nil
}

func serveWeb(ctx context.Context, env Env, ln net.Listener, o webOpts, dir string) int {
	token, err := web.NewToken(env.Rand)
	if err != nil {
		ln.Close()
		return exitErr(env, "", "", err)
	}
	host, port := webHostPort(ln, o.listen)
	pool := newToolPool(webIdleTimeout)
	defer pool.close()
	srv, err := web.New(web.Options{
		Backend: &webBackend{env: env, opts: o.run, dir: dir, pool: pool}, Token: token,
		Hosts: webHosts(ln.Addr().String(), host, port),
	})
	if err != nil {
		ln.Close()
		return exitErr(env, "", "", err)
	}
	warnListen(env, o.listen, host)
	fmt.Fprintf(env.Stderr, "http://%s/?token=%s\n", net.JoinHostPort(urlHost(host), port), token)
	if err := srv.Serve(ctx, ln); err != nil {
		return exitErr(env, "", "", err)
	}
	return 0
}

func webHostPort(ln net.Listener, listen string) (string, string) {
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		return "127.0.0.1", ""
	}
	if host, _, err := net.SplitHostPort(listen); err == nil {
		return host, port
	}
	return "127.0.0.1", port
}

func webHosts(bound, host, port string) []string {
	return append(web.AllowedHosts(bound), web.AllowedHosts(net.JoinHostPort(host, port))...)
}

func warnListen(env Env, listen, host string) {
	if listen == "" || isLoopback(host) {
		return
	}
	fmt.Fprintln(env.Stderr, "warning: not a loopback address, the token is sent without TLS")
	if isWildcard(host) {
		fmt.Fprintln(env.Stderr, "note: requests are accepted only for 127.0.0.1 and localhost, use a concrete --listen host for others")
	}
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isWildcard(host string) bool {
	if host == "" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsUnspecified()
}

func urlHost(host string) string {
	if isWildcard(host) {
		return "127.0.0.1"
	}
	return host
}
