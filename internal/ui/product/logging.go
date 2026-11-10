package product

/*
#include "logging.h"
*/
import "C"

func installLogging() func() {
	C.runix_logging_install()
	return func() { C.runix_logging_restore() }
}
