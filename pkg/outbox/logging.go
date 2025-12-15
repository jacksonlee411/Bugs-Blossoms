package outbox

import "github.com/sirupsen/logrus"

func logrusNop() *logrus.Entry {
	l := logrus.New()
	l.SetLevel(logrus.PanicLevel)
	return logrus.NewEntry(l)
}
