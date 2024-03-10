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

package apierror

import (
	"net/http"
	"strings"

	"github.com/palantir/go-baseapp/baseapp"
)

type ErrorResponse struct {
	Error string `json:"error"`
}

func WriteAPIError(w http.ResponseWriter, code int, message ...string) error {
	baseapp.WriteJSON(w, code, ErrorResponse{Error: strings.Join(message, "; ")})
	return nil
}
