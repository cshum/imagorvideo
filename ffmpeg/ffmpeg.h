#include <math.h>
#include <pthread.h>
#include <float.h>

#include <libavformat/avformat.h>
#include <libavcodec/avcodec.h>
#include <libavutil/opt.h>
#include <libavutil/file.h>
#include <libavutil/pixfmt.h>
#include <libswscale/swscale.h>
#include <libavutil/pixdesc.h>
#include <libavutil/intreadwrite.h>
#include <libavutil/imgutils.h>
#include <libavutil/display.h>

#define BUFFER_SIZE 1 << 12
#define READ_PACKET_FLAG 1
#define SEEK_PACKET_FLAG 2
#define INTERRUPT_FLAG 3
#define HAS_VIDEO_STREAM 1
#define HAS_AUDIO_STREAM 2
#define ERR_TOO_BIG FFERRTAG('H','M','M','M')

struct thumb_frame {
    AVFrame *frame;
    int *hist;
};

typedef struct ThumbContext {
    int n, alpha, max_frames;
    struct thumb_frame *frames;
    double *median;
    const AVPixFmtDescriptor *desc;
    size_t hist_size;
} ThumbContext;

int allocate_format_context(AVFormatContext **fmt_ctx);

int create_format_context(AVFormatContext *fmt_ctx, void* opaque, int callbacks);

void free_format_context(AVFormatContext *fmt_ctx);

void get_metadata(AVFormatContext *fmt_ctx, char **artist, char **title);

int find_streams(AVFormatContext *fmt_ctx, AVStream **video_stream, int *orientation);

int create_codec_context(AVStream *video_stream, AVCodecContext **dec_ctx);

AVFrame *convert_frame_to_rgb(AVFrame *frame, int alpha);

int encode_frame_to_image(AVFormatContext *fmt_ctx, AVFrame *frame, AVPacket *pkt);

AVPacket create_packet();

int
obtain_next_frame(AVFormatContext *fmt_ctx, AVCodecContext *dec_ctx, int stream_index, AVPacket *pkt, AVFrame **frame);

int64_t find_duration(AVFormatContext *fmt_ctx);

ThumbContext *create_thumb_context(AVStream *stream, AVFrame *frame);

void free_thumb_context(ThumbContext *thumb_ctx);

AVFrame *process_frames(ThumbContext *thumb_ctx);

void populate_histogram(ThumbContext *thumb_ctx, int n, AVFrame *frame);

extern int goPacketRead(void *opaque, uint8_t *buf, int buf_size);

extern int64_t goPacketSeek(void *opaque, int64_t seek, int whence);

extern int goInterrupt(void *opaque);