package genoa

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/urfave/cli/v3"
	"go.rtnl.ai/genoa/config"
	"go.rtnl.ai/genoa/errors"
	"go.rtnl.ai/x/rlog"
	"go.rtnl.ai/x/semver"
	"go.rtnl.ai/x/slugify"
	"sigs.k8s.io/yaml"
)

const (
	TTL = 10 * time.Second
)

var (
	commandRegistry map[string]Constructor
)

func Register(constructor Constructor) {
	cmd := constructor()
	kind := slugify.Slugify(cmd.Kind())
	if _, ok := commandRegistry[kind]; ok {
		panic(fmt.Errorf("%w: %q", errors.ErrAlreadyRegistered, kind))
	}
	commandRegistry[kind] = constructor
}

// Run is the main entry point for the genoa command.
func Run(ctx context.Context, c *cli.Command) (err error) {
	// Load the configuration from the environment.
	var conf config.Config
	if conf, err = config.Get(); err != nil {
		return errors.Exit(errors.ExitConfig, err)
	}

	// Setup logging and begin the genoa initialization.
	conf.SetupLogging()
	rlog.Info("starting genoa bootstrap", slog.String("defined_by", conf.ResourcePath), slog.String("version", Version(false)))

	var genoa *Genoa
	if genoa, err = Load(conf.ResourcePath); err != nil {
		return errors.Exit(errors.ExitGenoa, err)
	}

	commands := 0
	failures := 0
	for cmd := range genoa.Iter() {
		commands++
		rlog.Debug("running command", slog.Int("sequence", commands), slog.String("command", cmd.Kind()))

		if cerr := cmd.Run(ctx); cerr != nil {
			err = errors.Join(err, cerr)
			failures++
		}
	}

	if err != nil {
		rlog.Warn("genoa bootstrap completed with errors", slog.Int("commands", commands), slog.Int("failures", failures))
		return errors.Exit(errors.ExitCommand, err)
	}

	rlog.Debug("genoa bootstrap completed successfully", slog.Int("commands", commands))
	return nil
}

// Loads the Genoa resources from the given path as either JSON or YAML.
func Load(path string) (*Genoa, error) {
	switch ext := filepath.Ext(path); ext {
	case ".json":
		return LoadJSON(path)
	case ".yml", ".yaml":
		return LoadYAML(path)
	default:
		return nil, fmt.Errorf("unsupported file extension: %s", ext)
	}
}

func LoadJSON(path string) (genoa *Genoa, err error) {
	var file *os.File
	if file, err = os.Open(path); err != nil {
		return nil, err
	}
	defer file.Close()

	genoa = &Genoa{}
	if err = json.NewDecoder(file).Decode(genoa); err != nil {
		return nil, err
	}

	if err = genoa.load(); err != nil {
		return nil, err
	}

	return genoa, nil
}

func LoadYAML(path string) (genoa *Genoa, err error) {
	var file *os.File
	if file, err = os.Open(path); err != nil {
		return nil, err
	}
	defer file.Close()

	var buf []byte
	if buf, err = io.ReadAll(file); err != nil {
		return nil, err
	}

	var data []byte
	if data, err = yaml.YAMLToJSON(buf); err != nil {
		return nil, err
	}

	genoa = &Genoa{}
	if err = json.Unmarshal(data, genoa); err != nil {
		return nil, err
	}

	if err = genoa.load(); err != nil {
		return nil, err
	}

	return genoa, nil
}

// Genoa specifies the commands that need to be run and are defined in a JSON or YAML
// file. Generally speaking, a config map is loaded into the Kubernetes cluster and
// Genoa is executed from that config map. Alternatively, the YAML file can be created
// directly on the genoa pod and executed from there.
type Genoa struct {
	Version  string      `json:"version" yaml:"version"`
	Release  string      `json:"release" yaml:"release"`
	Commands []*Resource `json:"commands,omitempty" yaml:"commands,omitempty"`
	commands []Command
}

// Resource is a generic object that can be deserialized from the args into an object
// registered to the Kind specified (case insensitive).
type Resource struct {
	Kind string          `json:"kind" yaml:"kind"`
	Args json.RawMessage `json:"args" yaml:"args"`
}

func (g *Genoa) Iter() iter.Seq[Command] {
	return func(yield func(Command) bool) {
		for _, cmd := range g.commands {
			if !yield(cmd) {
				return
			}
		}
	}
}

// Load converts all of the resources into commands that can be run.
func (g *Genoa) load() (err error) {
	// Check the version of the Genoa file.
	var version semver.Version
	if version, err = semver.Parse(g.Version); err != nil {
		return errors.Join(errors.ErrInvalidVersion, fmt.Errorf("could not parse version %q: %w", g.Version, err))
	}

	if err = VersionCheck(version); err != nil {
		return errors.Join(errors.ErrInvalidVersion, err)
	}

	// Load the commands.
	g.commands = make([]Command, 0, len(g.Commands))
	for _, resource := range g.Commands {
		if constructor, ok := commandRegistry[slugify.Slugify(resource.Kind)]; ok {
			cmd := constructor()
			if err := json.Unmarshal(resource.Args, cmd); err != nil {
				return errors.Join(errors.ErrInvalidArgs, err)
			}
			g.commands = append(g.commands, cmd)
		} else {
			return fmt.Errorf("%w: %q", errors.ErrUnknownCommand, resource.Kind)
		}
	}
	return nil
}
