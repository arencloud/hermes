package db

import "testing"

func TestSummarizeSQL(t *testing.T){
	cases := []struct{ in, op, table string }{
		{"SELECT * FROM `users` WHERE id = ?", "SELECT", "users"},
		{"insert into providers (name) values (?)", "INSERT", "providers"},
		{"UPDATE buckets SET name = ? WHERE id = ?", "UPDATE", "buckets"},
	}
	for _, c := range cases {
		op, table := summarizeSQL(c.in)
		if op != c.op || table != c.table { t.Fatalf("summarizeSQL(%q)=%q,%q want %q,%q", c.in, op, table, c.op, c.table) }
	}
}
