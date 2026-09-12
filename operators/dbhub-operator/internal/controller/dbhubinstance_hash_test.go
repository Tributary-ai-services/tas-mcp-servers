/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import "testing"

// The credentials hash lands in the pod template, so it has to be stable for
// unchanged input and different whenever any DSN changes. A pod that keeps an
// obsolete DSN renders its config from a stale environment and never recovers,
// which is what kept dbhub down for 227 days.
func TestCalculateCredentialsHash(t *testing.T) {
	r := &DBHubInstanceReconciler{}

	base := map[string][]byte{
		"TAS_POSTGRES_DSN": []byte("postgres://user:pass@host:5432/db"),
		"ANALYTICS_DSN":    []byte("mysql://user:pass@host:3306/analytics"),
	}

	t.Run("deterministic across map iteration order", func(t *testing.T) {
		// Go randomises map iteration, so an unsorted implementation would
		// produce a fresh hash on most calls and roll the pods continuously.
		want := r.calculateCredentialsHash(base)
		for i := 0; i < 500; i++ {
			if got := r.calculateCredentialsHash(base); got != want {
				t.Fatalf("hash not deterministic: got %q, want %q", got, want)
			}
		}
	})

	t.Run("changes when a DSN is rotated", func(t *testing.T) {
		rotated := map[string][]byte{
			"TAS_POSTGRES_DSN": []byte("postgres://user:NEWPASS@host:5432/db"),
			"ANALYTICS_DSN":    []byte("mysql://user:pass@host:3306/analytics"),
		}
		if r.calculateCredentialsHash(rotated) == r.calculateCredentialsHash(base) {
			t.Error("rotating a DSN must change the hash, otherwise the pods are never rolled")
		}
	})

	t.Run("changes when a database is added", func(t *testing.T) {
		extra := map[string][]byte{
			"TAS_POSTGRES_DSN": []byte("postgres://user:pass@host:5432/db"),
			"ANALYTICS_DSN":    []byte("mysql://user:pass@host:3306/analytics"),
			"WAREHOUSE_DSN":    []byte("postgres://user:pass@host:5432/warehouse"),
		}
		if r.calculateCredentialsHash(extra) == r.calculateCredentialsHash(base) {
			t.Error("adding a database must change the hash")
		}
	})

	t.Run("encoding is unambiguous", func(t *testing.T) {
		// A plain "key=value\n" encoding collides on these pairs.
		if r.calculateCredentialsHash(map[string][]byte{"A": []byte("b=c")}) ==
			r.calculateCredentialsHash(map[string][]byte{"A=b": []byte("c")}) {
			t.Error(`{"A": "b=c"} and {"A=b": "c"} must not hash alike`)
		}
		if r.calculateCredentialsHash(map[string][]byte{"A": []byte("x\nB=y")}) ==
			r.calculateCredentialsHash(map[string][]byte{"A": []byte("x"), "B": []byte("y")}) {
			t.Error("a newline inside a value must not imitate a record boundary")
		}
	})

	t.Run("empty credentials hash to the empty-input hash", func(t *testing.T) {
		// e3b0c442... is sha256 of empty input. Seeing it on a live ReplicaSet
		// means the operator generated nothing, not that it generated a config.
		if got := r.calculateCredentialsHash(map[string][]byte{}); got != "e3b0c44298fc1c14" {
			t.Errorf("empty credentials hash = %q, want %q", got, "e3b0c44298fc1c14")
		}
	})
}
