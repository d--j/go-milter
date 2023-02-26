package milter

import (
	"reflect"
	"testing"
	"time"
)

func TestMacroBag_GetMacro(t *testing.T) {
	tests := []struct {
		name   string
		macros map[MacroName]string
		arg    MacroName
		want   string
	}{
		{"QueueID", map[MacroName]string{MacroQueueId: "123"}, MacroQueueId, "123"},
		{"QueueID empty", map[MacroName]string{MacroAuthAuthen: "123"}, MacroQueueId, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ltt := tt
			t.Parallel()
			m := &MacroBag{
				macros: ltt.macros,
			}
			if got := m.Get(ltt.arg); got != ltt.want {
				t.Errorf("Get() = %v, want %v", got, ltt.want)
			}
		})
	}
}

func TestMacroBag_GetMacroEx(t *testing.T) {
	tests := []struct {
		name      string
		macros    map[MacroName]string
		arg       MacroName
		wantValue string
		wantOk    bool
	}{
		{"QueueID", map[MacroName]string{MacroQueueId: "123"}, MacroQueueId, "123", true},
		{"QueueID 2", map[MacroName]string{MacroAuthSsf: "456", MacroQueueId: "123"}, MacroQueueId, "123", true},
		{"QueueID empty", map[MacroName]string{MacroAuthAuthen: "123"}, MacroQueueId, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ltt := tt
			t.Parallel()
			m := &MacroBag{
				macros: ltt.macros,
			}
			gotValue, gotOk := m.GetEx(ltt.arg)
			if gotValue != ltt.wantValue {
				t.Errorf("GetEx() gotValue = %v, want %v", gotValue, ltt.wantValue)
			}
			if gotOk != ltt.wantOk {
				t.Errorf("GetEx() gotOk = %v, want %v", gotOk, ltt.wantOk)
			}
		})
	}
}

func TestMacroBag_GetMacroEx_Dates(t *testing.T) {
	t.Parallel()
	type dates struct {
		current time.Time
		header  time.Time
	}
	date1 := time.Date(2023, time.January, 1, 1, 1, 1, 0, time.UTC)
	tests := []struct {
		name      string
		dates     dates
		macros    map[MacroName]string
		arg       MacroName
		wantValue string
		wantOk    bool
	}{
		{"header: force set", dates{header: date1}, map[MacroName]string{MacroDateRFC822Origin: "123"}, MacroDateRFC822Origin, "123", true},
		{"header: set", dates{header: date1}, map[MacroName]string{}, MacroDateRFC822Origin, "01 Jan 23 01:01 +0000", true},
		{"header: not-set", dates{}, map[MacroName]string{}, MacroDateRFC822Origin, "", false},
		{"current: force set", dates{current: date1}, map[MacroName]string{MacroDateRFC822Current: "123"}, MacroDateRFC822Current, "123", true},
		{"current: set", dates{current: date1}, map[MacroName]string{}, MacroDateRFC822Current, "01 Jan 23 01:01 +0000", true},
		{"current: set seconds", dates{current: date1}, map[MacroName]string{}, MacroDateSecondsCurrent, "1672534861", true},
		{"current: set ANSI", dates{current: date1}, map[MacroName]string{}, MacroDateANSICCurrent, "Sun Jan  1 01:01:01 2023", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ltt := tt
			t.Parallel()
			m := &MacroBag{
				macros: ltt.macros,
			}
			m.SetHeaderDate(ltt.dates.header)
			m.SetCurrentDate(ltt.dates.current)
			gotValue, gotOk := m.GetEx(ltt.arg)
			if gotValue != ltt.wantValue {
				t.Errorf("GetEx() gotValue = %v, want %v", gotValue, ltt.wantValue)
			}
			if gotOk != ltt.wantOk {
				t.Errorf("GetEx() gotOk = %v, want %v", gotOk, ltt.wantOk)
			}
		})
	}
	t.Run("current: not-set", func(t *testing.T) {
		m := &MacroBag{
			macros: map[MacroName]string{},
		}
		gotValue, gotOk := m.GetEx(MacroDateRFC822Current)
		if gotValue == "" {
			t.Errorf("GetEx() gotValue = %v, want not empty", gotValue)
		}
		if gotOk != true {
			t.Errorf("GetEx() gotOk = %v, want %v", gotOk, true)
		}
	})
}

func TestMacroBag_SetMacro(t *testing.T) {
	type args struct {
		name  MacroName
		value string
	}
	tests := []struct {
		name   string
		macros map[MacroName]string
		args   args
	}{
		{"Overwrite", map[MacroName]string{MacroQueueId: "123"}, args{MacroQueueId, "456"}},
		{"Set", map[MacroName]string{}, args{MacroQueueId, "456"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ltt := tt
			t.Parallel()
			m := &MacroBag{
				macros: ltt.macros,
			}
			m.Set(ltt.args.name, ltt.args.value)
			if got := m.Get(ltt.args.name); got != ltt.args.value {
				t.Errorf("Get() = %v, want %v", got, ltt.args.value)
			}
		})
	}
}

func TestMacroBag_Copy(t *testing.T) {
	type fields struct {
		macros      map[MacroName]string
		currentDate time.Time
		headerDate  time.Time
	}
	tests := []struct {
		name   string
		fields fields
		want   map[MacroName]string
	}{
		{"empty", fields{}, map[MacroName]string{}},
		{"simple", fields{macros: map[MacroName]string{MacroQueueId: "123"}}, map[MacroName]string{MacroQueueId: "123"}},
		{"no-dates", fields{macros: map[MacroName]string{MacroQueueId: "123"}, headerDate: time.Now(), currentDate: time.Now()}, map[MacroName]string{MacroQueueId: "123"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &MacroBag{
				macros:      tt.fields.macros,
				currentDate: tt.fields.currentDate,
				headerDate:  tt.fields.headerDate,
			}
			if got := m.Copy().macros; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Copy() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestMacroReader_Get(t *testing.T) {
	tests := []struct {
		name         string
		macrosStages *macrosStages
		arg          MacroName
		want         string
	}{
		{"QueueID last", &macrosStages{[]map[MacroName]string{nil, nil, nil, nil, nil, nil, nil, {MacroQueueId: "123"}}}, MacroQueueId, "123"},
		{"QueueID first", &macrosStages{[]map[MacroName]string{{MacroQueueId: "123"}, nil, nil, nil, nil, nil, nil, nil}}, MacroQueueId, "123"},
		{"QueueID middle", &macrosStages{[]map[MacroName]string{nil, nil, nil, {MacroQueueId: "123"}, nil, nil, nil, nil}}, MacroQueueId, "123"},
		{"QueueID nil", &macrosStages{[]map[MacroName]string{nil, nil, nil, nil, nil, nil, nil, nil}}, MacroQueueId, ""},
		{"QueueID priority", &macrosStages{[]map[MacroName]string{{MacroQueueId: "456"}, nil, nil, nil, nil, nil, {MacroQueueId: "123"}, nil}}, MacroQueueId, "123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ltt := tt
			t.Parallel()
			r := &macroReader{
				macrosStages: ltt.macrosStages,
			}
			if got := r.Get(ltt.arg); got != ltt.want {
				t.Errorf("Get() = %v, want %v", got, ltt.want)
			}
		})
	}
}

func TestMacroReader_GetEx(t *testing.T) {
	tests := []struct {
		name         string
		macrosStages *macrosStages
		arg          MacroName
		want         string
		want1        bool
	}{
		{"QueueID last", &macrosStages{[]map[MacroName]string{nil, nil, nil, nil, nil, nil, nil, {MacroQueueId: "123"}}}, MacroQueueId, "123", true},
		{"QueueID first", &macrosStages{[]map[MacroName]string{{MacroQueueId: "123"}, nil, nil, nil, nil, nil, nil, nil}}, MacroQueueId, "123", true},
		{"QueueID middle", &macrosStages{[]map[MacroName]string{nil, nil, nil, {MacroQueueId: "123"}, nil, nil, nil, nil}}, MacroQueueId, "123", true},
		{"QueueID nil", &macrosStages{[]map[MacroName]string{nil, nil, nil, nil, nil, nil, nil, nil}}, MacroQueueId, "", false},
		{"QueueID priority", &macrosStages{[]map[MacroName]string{{MacroQueueId: "456"}, nil, nil, nil, nil, nil, {MacroQueueId: "123"}, nil}}, MacroQueueId, "123", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ltt := tt
			t.Parallel()
			r := &macroReader{
				macrosStages: ltt.macrosStages,
			}
			got, got1 := r.GetEx(ltt.arg)
			if got != ltt.want {
				t.Errorf("GetEx() got = %v, want %v", got, ltt.want)
			}
			if got1 != ltt.want1 {
				t.Errorf("GetEx() got1 = %v, want %v", got1, ltt.want1)
			}
		})
	}
}

func Test_macrosStages_DelMacro(t *testing.T) {
	type args struct {
		stage MacroStage
		name  MacroName
	}
	tests := []struct {
		name     string
		byStages []map[MacroName]string
		args     args
	}{
		{"empty", []map[MacroName]string{nil, nil, nil, nil, nil, nil, nil, nil}, args{StageConnect, MacroQueueId}},
		{"simple", []map[MacroName]string{{MacroQueueId: "123"}, nil, nil, nil, nil, nil, nil, nil}, args{StageConnect, MacroQueueId}},
		{"multiple", []map[MacroName]string{{MacroQueueId: "123"}, {MacroQueueId: "123"}, {MacroQueueId: "123"}, {MacroQueueId: "123"}, nil, nil, nil, nil}, args{StageConnect, MacroQueueId}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ltt := tt
			t.Parallel()
			s := &macrosStages{
				byStages: tt.byStages,
			}
			s.DelMacro(ltt.args.stage, ltt.args.name)
			if _, st := s.GetMacroEx(ltt.args.name); st == ltt.args.stage {
				t.Errorf("DelMacro() did not delete %v in stage %v", ltt.args.name, ltt.args.stage)
			}
		})
	}
}

func Test_macrosStages_DelStage(t *testing.T) {
	tests := []struct {
		name     string
		byStages []map[MacroName]string
		stage    MacroStage
	}{
		{"noop", []map[MacroName]string{nil, nil, nil, nil, nil, nil, nil}, StageConnect},
		{"empty", []map[MacroName]string{{}, {}, {}, {}, {}, {}, {}}, StageConnect},
		{"connect", []map[MacroName]string{{MacroQueueId: "123"}, {}, {}, {}, {}, {}, {}}, StageConnect},
		{"helo", []map[MacroName]string{{}, {MacroQueueId: "123"}, {}, {}, {}, {}, {}}, StageHelo},
		{"mail", []map[MacroName]string{{}, {}, {MacroQueueId: "123"}, {}, {}, {}, {}}, StageMail},
		{"rcpt", []map[MacroName]string{{}, {}, {}, {MacroQueueId: "123"}, {}, {}, {}}, StageRcpt},
		{"data", []map[MacroName]string{{}, {}, {}, {}, {MacroQueueId: "123"}, {}, {}}, StageData},
		{"EOM", []map[MacroName]string{{}, {}, {}, {}, {}, {MacroQueueId: "123"}, {}}, StageEOM},
		{"EOH", []map[MacroName]string{{}, {}, {}, {}, {}, {}, {MacroQueueId: "123"}}, StageEOH},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ltt := tt
			t.Parallel()
			s := &macrosStages{
				byStages: ltt.byStages,
			}
			s.DelStage(ltt.stage)
			if s.byStages[ltt.stage] != nil {
				t.Errorf("DelStage() did not delete stage %v", ltt.stage)
			}
		})
	}
}

func Test_macrosStages_DelStageAndAbove(t *testing.T) {
	tests := []struct {
		name     string
		byStages []map[MacroName]string
		stage    MacroStage
	}{
		{"noop", []map[MacroName]string{nil, nil, nil, nil, nil, nil, nil, nil}, StageConnect},
		{"empty", []map[MacroName]string{{}, {}, {}, {}, {}, {}, {}, {}}, StageConnect},
		{"connect", []map[MacroName]string{{MacroQueueId: "123"}, {}, {}, {}, {}, {}, {}, {}}, StageConnect},
		{"helo", []map[MacroName]string{{}, {MacroQueueId: "123"}, {}, {}, {}, {}, {}, {}}, StageHelo},
		{"mail", []map[MacroName]string{{}, {}, {MacroQueueId: "123"}, {}, {}, {}, {}, {}}, StageMail},
		{"rcpt", []map[MacroName]string{{}, {}, {}, {MacroQueueId: "123"}, {}, {}, {}, {}}, StageRcpt},
		{"data", []map[MacroName]string{{}, {}, {}, {}, {MacroQueueId: "123"}, {}, {}, {}}, StageData},
		{"EOM", []map[MacroName]string{{}, {}, {}, {}, {}, {MacroQueueId: "123"}, {}, {}}, StageEOM},
		{"EOH", []map[MacroName]string{{}, {}, {}, {}, {}, {}, {MacroQueueId: "123"}, {}}, StageEOH},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ltt := tt
			t.Parallel()
			s := &macrosStages{
				byStages: ltt.byStages,
			}
			s.DelStageAndAbove(ltt.stage)
			if ltt.stage == StageEOH {
				if s.byStages[StageEOH] != nil {
					t.Errorf("DelStageAndAbove() did not delete stage %v", StageEOH)
				}
				if s.byStages[StageEOM] != nil {
					t.Errorf("DelStageAndAbove() did not delete stage %v", StageEOM)
				}
			} else if ltt.stage == StageEOM {
				if s.byStages[StageEOM] != nil {
					t.Errorf("DelStageAndAbove() did not delete stage %v", StageEOM)
				}
			} else {
				for st := ltt.stage; st < StageEndMarker; st += 1 {
					if s.byStages[st] != nil {
						t.Errorf("DelStageAndAbove() did not delete stage %v", st)
					}
				}
			}
		})
	}
}

func Test_macrosStages_GetMacroEx(t *testing.T) {
	type fields struct {
		byStages []map[MacroName]string
	}
	type args struct {
		name MacroName
	}
	tests := []struct {
		name           string
		fields         fields
		args           args
		wantValue      string
		wantStageFound MacroStage
	}{
		{"empty", fields{[]map[MacroName]string{nil, nil, nil, nil, nil, nil, nil, nil}}, args{MacroQueueId}, "", StageNotFoundMarker},
		{"first", fields{[]map[MacroName]string{{MacroQueueId: "123"}, nil, nil, nil, nil, nil, nil, nil}}, args{MacroQueueId}, "123", StageConnect},
		{"last", fields{[]map[MacroName]string{nil, nil, nil, nil, nil, nil, nil, {MacroQueueId: "123"}}}, args{MacroQueueId}, "123", StageEndMarker},
		{"last1", fields{[]map[MacroName]string{{MacroQueueId: "123"}, nil, nil, nil, nil, nil, nil, {MacroQueueId: "123"}}}, args{MacroQueueId}, "123", StageEndMarker},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ltt := tt
			t.Parallel()
			s := &macrosStages{
				byStages: ltt.fields.byStages,
			}
			gotValue, gotStageFound := s.GetMacroEx(ltt.args.name)
			if gotValue != ltt.wantValue {
				t.Errorf("GetEx() gotValue = %v, want %v", gotValue, ltt.wantValue)
			}
			if gotStageFound != ltt.wantStageFound {
				t.Errorf("GetEx() gotStageFound = %v, want %v", gotStageFound, ltt.wantStageFound)
			}
		})
	}
}

func Test_macrosStages_SetMacro(t *testing.T) {
	type fields struct {
		byStages []map[MacroName]string
	}
	type args struct {
		stage MacroStage
		name  MacroName
		value string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		{"nil", fields{[]map[MacroName]string{nil, nil, nil, nil, nil, nil, nil}}, args{StageConnect, MacroQueueId, "123"}},
		{"empty", fields{[]map[MacroName]string{{}, nil, nil, nil, nil, nil, nil}}, args{StageConnect, MacroQueueId, "123"}},
		{"overwrite", fields{[]map[MacroName]string{{MacroQueueId: "456"}, nil, nil, nil, nil, nil, nil}}, args{StageConnect, MacroQueueId, "123"}},
		{"last", fields{[]map[MacroName]string{{}, nil, nil, nil, nil, nil, {}}}, args{StageEOM, MacroQueueId, "123"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ltt := tt
			t.Parallel()
			s := &macrosStages{
				byStages: ltt.fields.byStages,
			}
			s.SetMacro(ltt.args.stage, ltt.args.name, ltt.args.value)
			if s.byStages[ltt.args.stage][ltt.args.name] != ltt.args.value {
				t.Errorf("Set() did not set the correct value = %v, want %v", s.byStages[ltt.args.stage][ltt.args.name], ltt.args.value)
			}
		})
	}
}

func Test_macrosStages_SetStage(t *testing.T) {
	type fields struct {
		byStages []map[MacroName]string
	}
	type args struct {
		stage MacroStage
		kv    []string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		wants  map[MacroName]string
	}{
		{"empty", fields{[]map[MacroName]string{nil, nil, nil, nil, nil, nil, nil}}, args{StageConnect, []string{}}, map[MacroName]string{}},
		{"simple nil", fields{[]map[MacroName]string{nil, nil, nil, nil, nil, nil, nil}}, args{StageConnect, []string{MacroQueueId, "123"}}, map[MacroName]string{MacroQueueId: "123"}},
		{"simple empty", fields{[]map[MacroName]string{{}, {}, {}, {}, {}, {}, {}}}, args{StageConnect, []string{MacroQueueId, "123"}}, map[MacroName]string{MacroQueueId: "123"}},
		{"multiple", fields{[]map[MacroName]string{{}, {}, {}, {}, {}, {}, {}}}, args{StageConnect, []string{MacroQueueId, "123", MacroAuthAuthen, "123"}}, map[MacroName]string{MacroQueueId: "123", MacroAuthAuthen: "123"}},
		{"overwrite", fields{[]map[MacroName]string{{MacroAuthAuthen: "123"}, {}, {}, {}, {}, {}, {}}}, args{StageConnect, []string{MacroQueueId, "123"}}, map[MacroName]string{MacroQueueId: "123"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ltt := tt
			t.Parallel()
			s := &macrosStages{
				byStages: ltt.fields.byStages,
			}
			s.SetStage(ltt.args.stage, ltt.args.kv...)
			if !reflect.DeepEqual(s.byStages[ltt.args.stage], ltt.wants) {
				t.Errorf("SetStage() result = %v, want %v", s.byStages[ltt.args.stage], ltt.wants)
			}
		})
	}
}

func Test_macrosStages_SetStageMap(t *testing.T) {
	type fields struct {
		byStages []map[MacroName]string
	}
	type args struct {
		stage MacroStage
		kv    map[MacroName]string
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		wants  map[MacroName]string
	}{
		{"empty", fields{[]map[MacroName]string{nil, nil, nil, nil, nil, nil, nil}}, args{StageConnect, map[MacroName]string{}}, map[MacroName]string{}},
		{"simple nil", fields{[]map[MacroName]string{nil, nil, nil, nil, nil, nil, nil}}, args{StageConnect, map[MacroName]string{MacroQueueId: "123"}}, map[MacroName]string{MacroQueueId: "123"}},
		{"simple empty", fields{[]map[MacroName]string{{}, {}, {}, {}, {}, {}, {}}}, args{StageConnect, map[MacroName]string{MacroQueueId: "123"}}, map[MacroName]string{MacroQueueId: "123"}},
		{"multiple", fields{[]map[MacroName]string{{}, {}, {}, {}, {}, {}, {}}}, args{StageConnect, map[MacroName]string{MacroQueueId: "123", MacroAuthAuthen: "123"}}, map[MacroName]string{MacroQueueId: "123", MacroAuthAuthen: "123"}},
		{"overwrite", fields{[]map[MacroName]string{{MacroAuthAuthen: "123"}, {}, {}, {}, {}, {}, {}}}, args{StageConnect, map[MacroName]string{MacroQueueId: "123"}}, map[MacroName]string{MacroQueueId: "123"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ltt := tt
			t.Parallel()
			s := &macrosStages{
				byStages: ltt.fields.byStages,
			}
			s.SetStageMap(ltt.args.stage, ltt.args.kv)
			if !reflect.DeepEqual(s.byStages[ltt.args.stage], ltt.wants) {
				t.Errorf("SetStageMap() result = %v, want %v", s.byStages[ltt.args.stage], ltt.wants)
			}
		})
	}
}

func Test_newMacroStages(t *testing.T) {
	t.Parallel()
	got := newMacroStages()
	if len(got.byStages) != int(StageEndMarker)+1 {
		t.Errorf("newMacroStages() len(byStages) = %d, want %d", len(got.byStages)+1, StageEndMarker)
	}
}

func Test_parseRequestedMacros(t *testing.T) {
	tests := []struct {
		name string
		str  string
		want []string
	}{
		{"empty", "", []string{}},
		{"spaces", "   \t,,", []string{}},
		{"single", "{auth_authen}", []string{"{auth_authen}"}},
		{"single2", "  {auth_authen},  ", []string{"{auth_authen}"}},
		{"multiple", "  {auth_authen}, {auth_authen} j ", []string{"{auth_authen}", "{auth_authen}", "j"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ltt := tt
			t.Parallel()
			if got := parseRequestedMacros(ltt.str); !reflect.DeepEqual(got, ltt.want) {
				t.Errorf("parseRequestedMacros() = %v, want %v", got, ltt.want)
			}
		})
	}
}

func Test_removeDuplicates(t *testing.T) {
	tests := []struct {
		name string
		str  []string
		want []string
	}{
		{"empty", []string{}, []string{}},
		{"nil", nil, []string{}},
		{"beginning", []string{"a", "a", "b"}, []string{"a", "b"}},
		{"end", []string{"a", "b", "b"}, []string{"a", "b"}},
		{"single", []string{"a"}, []string{"a"}},
		{"multiple", []string{"a", "b", "a", "a"}, []string{"a", "b"}},
		{"multiple2", []string{"b", "a", "b", "a", "a"}, []string{"b", "a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ltt := tt
			t.Parallel()
			if got := removeDuplicates(ltt.str); !reflect.DeepEqual(got, ltt.want) {
				t.Errorf("removeDuplicates() = %v, want %v", got, ltt.want)
			}
		})
	}
}

func Test_removeEmpty(t *testing.T) {
	tests := []struct {
		name string
		str  []string
		want []string
	}{
		{"empty", []string{}, []string{}},
		{"nil", nil, []string{}},
		{"beginning", []string{"", "a", "b"}, []string{"a", "b"}},
		{"end", []string{"a", "b", ""}, []string{"a", "b"}},
		{"single", []string{""}, []string{}},
		{"multiple", []string{"a", "", "b", ""}, []string{"a", "b"}},
		{"multiple2", []string{"", "", "b", "a", ""}, []string{"b", "a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ltt := tt
			t.Parallel()
			if got := removeEmpty(ltt.str); !reflect.DeepEqual(got, ltt.want) {
				t.Errorf("removeEmpty() = %v, want %v", got, ltt.want)
			}
		})
	}
}
