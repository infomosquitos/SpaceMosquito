package sqlite_test

import "testing"

func TestUserNames(t *testing.T) {
	db, ctx := openTestDB(t)
	defer db.Close()
	if err := db.SaveUserNames(ctx, map[string]string{"712020:aa": "Ann", "712020:bb": "Bob", "x": ""}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveUserNames(ctx, map[string]string{"712020:bb": "Bob Builder"}); err != nil {
		t.Fatal(err)
	}
	got, err := db.UserNames(ctx)
	if err != nil || len(got) != 2 || got["712020:aa"] != "Ann" || got["712020:bb"] != "Bob Builder" {
		t.Fatalf("names = %v, err %v", got, err)
	}
}
