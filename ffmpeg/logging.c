#include "logging.h"

void goavLogCallback(void *class_ptr, int level, const char *fmt, va_list vl) {
    char line[LINE_SZ];
    int print_prefix = 1;
    av_log_format_line(class_ptr, level, fmt, vl, line, LINE_SZ, &print_prefix);
    goAVLoggingHandler(level, line);
}

void goavLogSetup() {
    av_log_set_callback(goavLogCallback);
}
