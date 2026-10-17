package service

import (
	"errors"
	"testing"

	xuexitongApi "github.com/yatori-dev/yatori-go-core/api/xuexitong"
)

func TestXXTExtractExamJumpURL(t *testing.T) {
	html := `<input type="hidden" id="examJumpUrl" value="/exam-ans/exam/phone/task-exam?courseId=1&amp;code=t8918554" />`
	got := xxtExtractExamJumpURL(html)
	want := `/exam-ans/exam/phone/task-exam?courseId=1&code=t8918554`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestXXTExtractExamCodeDoesNotTreatTypeAsCode(t *testing.T) {
	raw := `https://mooc1-api.chaoxing.com/exam-ans/android/mtaskmsgspecial?taskrefId=10808239&type=exam&code=`
	if got, src := xxtExtractExamCode(raw); got != "" || src != "" {
		t.Fatalf("unexpected code %q src %q", got, src)
	}
}

func TestXXTExtractExamCodeFromExplicitCode(t *testing.T) {
	raw := `window.examConfig = { examCode: "t8918554" }`
	got, src := xxtExtractExamCode(raw)
	if got != "t8918554" || src == "" {
		t.Fatalf("got %q src %q", got, src)
	}
}

func TestXXTExtractExamQuestionTotal(t *testing.T) {
	cases := []struct {
		html string
		want int
	}{
		{`<div aria-label='当前第1 题; 共6题'><span>1/6</span></div>`, 6},
		{`<h3>单选题（共6题，30.0分）</h3>`, 6},
		{`<span>1/12</span>`, 12},
	}
	for _, tc := range cases {
		if got := xxtExtractExamQuestionTotal(tc.html); got != tc.want {
			t.Fatalf("got %d want %d for %q", got, tc.want, tc.html)
		}
	}
}

func TestXXTSubmitResponseFailed(t *testing.T) {
	failed := []string{
		`{"status":false,"msg":"enc error!"}`,
		`{"success":false}`,
		`提交失败，请重试`,
		`enc error`,
		``,
	}
	for _, raw := range failed {
		if !xxtSubmitResponseFailed(raw) {
			t.Errorf("expected failed response: %q", raw)
		}
	}
	passed := []string{`{"status":true}`, `{"success":true,"msg":"ok"}`, `ok`}
	for _, raw := range passed {
		if xxtSubmitResponseFailed(raw) {
			t.Errorf("expected successful response: %q", raw)
		}
	}
}

func TestXXTCaptchaErrorDoesNotLookLikeExamCode(t *testing.T) {
	if !isXXTCaptchaError(errors.New("考试滑块验证码处理失败")) {
		t.Fatal("slider failure was not classified as captcha")
	}
	if isXXTCaptchaError(errors.New("考试码错误")) {
		t.Fatal("exam code error was classified as captcha")
	}
}

func TestCopyChapterQuestionAnswersKeepsFreshMetadata(t *testing.T) {
	src := xuexitongApi.Question{
		JobId:  "old-job",
		Choice: []xuexitongApi.ChoiceQue{{Qid: "c1", Answers: []string{"A"}}},
		Fill:   []xuexitongApi.FillQue{{Qid: "f1", OpFromAnswer: []string{"hello"}}},
	}
	dst := xuexitongApi.Question{
		JobId:  "fresh-job",
		Choice: []xuexitongApi.ChoiceQue{{Qid: "c1"}},
		Fill:   []xuexitongApi.FillQue{{Qid: "f1"}},
	}
	copyChapterQuestionAnswers(&dst, &src)
	if dst.JobId != "fresh-job" || len(dst.Choice[0].Answers) != 1 || dst.Choice[0].Answers[0] != "A" {
		t.Fatalf("fresh metadata or choice answer was lost: %+v", dst)
	}
	if got := dst.Fill[0].OpFromAnswer; len(got) != 1 || got[0] != "hello" {
		t.Fatalf("fill answer was not copied: %+v", dst.Fill[0].OpFromAnswer)
	}
	src.Choice[0].Answers[0] = "B"
	if dst.Choice[0].Answers[0] != "A" {
		t.Fatal("answer copy aliases source slice")
	}
}
