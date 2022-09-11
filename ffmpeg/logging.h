#include <stdarg.h>

#define LINE_SZ 1024

extern void goAVLoggingHandler(int level, char *str);
extern void av_log_set_callback(void (*callback)(void *, int, const char *, va_list));
extern void av_log_format_line(void *ptr, int level, const char *fmt, va_list vl, char *line, int line_size, int *print_prefix);

void goavLogCallback(void *class_ptr, int level, const char *fmt, va_list vl);

void goavLogSetup();