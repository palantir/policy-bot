// Copyright 2018 Palantir Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"io/ioutil"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/palantir/policy-bot/server"
)

var serverCmdConfig struct {
	Path string
}

var ServerCmd = &cobra.Command{
	Use:   "server",
	Short: "Runs policy-bot in server mode.",
	Long:  "Runs policy-bot in a long-running server mode, receiving and responding to webhooks.",

	RunE: serverCmd,
}

func readServerConfig(cfgFile string) (*server.Config, error) {
	fi, err := os.Stat(cfgFile)
	if err != nil {
		return nil, errors.Wrapf(err, "failed fetching server config file: %s", cfgFile)
	}
	if os.IsNotExist(err) {
		return nil, errors.Wrapf(err, "server config file does not exist: %s", cfgFile)
	}
	if !fi.Mode().IsRegular() {
		return nil, errors.New("server config file is not a regular file: " + cfgFile)
	}

	var bytes []byte
	bytes, err = ioutil.ReadFile(cfgFile)
	if err != nil {
		return nil, errors.Wrapf(err, "failed reading server config file: %s", cfgFile)
	}

	cfg, err := server.ParseConfig(bytes)
	if err != nil {
		return nil, errors.Wrapf(err, "failed parsing server config")
	}

	return cfg, nil
}

func serverCmd(cmd *cobra.Command, args []string) error {
	cfg, err := readServerConfig(serverCmdConfig.Path)
	if err != nil {
		return errors.Wrapf(err, "failed to read server config")
	}

	s, err := server.New(cfg)
	if err != nil {
		return err
	}

	return errors.Wrap(s.Start(), "server terminated")
}

func init() {
	RootCmd.AddCommand(ServerCmd)

	ServerCmd.Flags().StringVarP(&serverCmdConfig.Path, "config", "c", "var/conf/policy-bot.yml", "configuration file for policy-bot")
}
