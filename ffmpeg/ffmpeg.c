#include "ffmpeg.h"

pthread_mutex_t mutex = PTHREAD_MUTEX_INITIALIZER;

void free_format_context(AVFormatContext *fmt_ctx) {
    if (!fmt_ctx) {
        return;
    }
    av_free(fmt_ctx->pb->buffer);
    avio_context_free(&fmt_ctx->pb);
    avformat_close_input(&fmt_ctx);
}

int allocate_format_context(AVFormatContext **fmt_ctx) {
    AVFormatContext *ctx = NULL;
    if (!(ctx = avformat_alloc_context())) {
        return AVERROR(ENOMEM);
    }
    *fmt_ctx = ctx;
    return 0;
}

int create_format_context(AVFormatContext *fmt_ctx, void* opaque, int flags) {
    int err = 0;
    uint8_t *avio_buffer = NULL;
    AVIOContext *avio_ctx = NULL;
    if (!(avio_buffer = av_malloc(BUFFER_SIZE))) {
        avformat_free_context(fmt_ctx);
        return AVERROR(ENOMEM);
    }
    void *reader = NULL;
    void *seeker = NULL;
    int write_flag = 0;
    int seekable = 0;
    if (flags & READ_PACKET_FLAG) {
        reader = goPacketRead;
    }
    if (flags & SEEK_PACKET_FLAG) {
        seeker = goPacketSeek;
        seekable = 1;
    }
    if (!(avio_ctx = avio_alloc_context(avio_buffer, BUFFER_SIZE, write_flag, opaque, reader, NULL, seeker))) {
        av_free(avio_buffer);
        avformat_free_context(fmt_ctx);
        return AVERROR(ENOMEM);
    }
    fmt_ctx->pb = avio_ctx;
    fmt_ctx->pb->seekable = seekable;
    err = avformat_open_input(&fmt_ctx, NULL, NULL, NULL);
    if (err < 0) {
        av_free(avio_ctx->buffer);
        avio_context_free(&avio_ctx);
        free_format_context(fmt_ctx);
        return err;
    }
    err = pthread_mutex_lock(&mutex);
    if (err < 0) {
        free_format_context(fmt_ctx);
        return err;
    }
    err = avformat_find_stream_info(fmt_ctx, NULL);
    int muErr = pthread_mutex_unlock(&mutex);
    if (err < 0 || muErr < 0) {
        free_format_context(fmt_ctx);
        if (muErr < 0) {
            return muErr;
        }
    }
    return err;
}

static int get_orientation(AVStream *video_stream) {
    uint8_t *display_matrix = av_stream_get_side_data(video_stream, AV_PKT_DATA_DISPLAYMATRIX, NULL);
    double theta = 0;
    if (display_matrix) {
        theta = -av_display_rotation_get((int32_t *) display_matrix);
    }

    theta -= 360 * floor(theta / 360 + 0.9 / 360);

    int rot = (int) (90 * round(theta / 90)) % 360;

    switch (rot) {
        case 90:
            return 6;
        case 180:
            return 3;
        case 270:
            return 8;
        default:
            return 1;
    };
}

void get_metadata(AVFormatContext *fmt_ctx, char **artist, char **title) {
    AVDictionaryEntry *tag = NULL;
    if ((tag = av_dict_get(fmt_ctx->metadata, "artist", NULL, 0))) {
        *artist = tag->value;
    }
    if ((tag = av_dict_get(fmt_ctx->metadata, "title", NULL, 0))) {
        *title = tag->value;
    }
}

int find_streams(AVFormatContext *fmt_ctx, AVStream **video_stream, int *orientation) {
    int video_stream_index = av_find_best_stream(fmt_ctx, AVMEDIA_TYPE_VIDEO, -1, -1, NULL, 0);
    int audio_stream_index = av_find_best_stream(fmt_ctx, AVMEDIA_TYPE_AUDIO, -1, -1, NULL, 0);
    int video_audio = 0;
    if (audio_stream_index >= 0) {
        video_audio |= HAS_AUDIO_STREAM;
    }
    if (video_stream_index >= 0) {
        video_audio |= HAS_VIDEO_STREAM;
    } else {
        if (video_audio) {
            return video_audio;
        }
        return AVERROR_STREAM_NOT_FOUND;
    }
    *video_stream = fmt_ctx->streams[video_stream_index];
    *orientation = get_orientation(*video_stream);
    return video_audio;
}

static int open_codec(AVCodecContext *codec_ctx, AVCodec *codec) {
    int err = pthread_mutex_lock(&mutex);
    if (err < 0) {
        return err;
    }
    err = avcodec_open2(codec_ctx, codec, NULL);
    int muErr = pthread_mutex_unlock(&mutex);
    if (muErr < 0) {
        return muErr;
    }
    return err;
}

int create_codec_context(AVStream *video_stream, AVCodecContext **dec_ctx) {
    AVCodec *dec = NULL;
    AVCodecParameters *par = video_stream->codecpar;
    if (par->codec_id == AV_CODEC_ID_VP8) {
        dec = avcodec_find_decoder_by_name("libvpx");
    } else if (par->codec_id == AV_CODEC_ID_VP9) {
        dec = avcodec_find_decoder_by_name("libvpx-vp9");
    }
    if (!dec) {
        dec = avcodec_find_decoder(par->codec_id);
    }
    if (dec == NULL) {
        return AVERROR_DECODER_NOT_FOUND;
    }
    if (par->format == -1) {
        return AVERROR_INVALIDDATA;
    }
    if (av_get_bits_per_pixel(av_pix_fmt_desc_get(par->format)) * par->height * par->width > 1 << 30) {
        return ERR_TOO_BIG;
    }
    if (!(*dec_ctx = avcodec_alloc_context3(dec))) {
        return AVERROR(ENOMEM);
    }
    int err = avcodec_parameters_to_context(*dec_ctx, par);
    if (err < 0) {
        avcodec_free_context(dec_ctx);
        return err;
    }
    err = open_codec(*dec_ctx, dec);
    if (err < 0) {
        avcodec_free_context(dec_ctx);
    }
    return err;
}

AVFrame *convert_frame_to_rgb(AVFrame *frame, int alpha) {
    int output_fmt = alpha ? AV_PIX_FMT_RGBA : AV_PIX_FMT_RGB24;
    struct SwsContext *sws_ctx = NULL;
    AVFrame *output_frame = av_frame_alloc();
    if (!output_frame) {
        return output_frame;
    }
    output_frame->height = frame->height;
    output_frame->width = frame->width;
    output_frame->format = output_fmt;
    if (av_frame_get_buffer(output_frame, 1) < 0) {
        goto free;
    }
    if (output_fmt == frame->format) {
        if (av_frame_copy(output_frame, frame) < 0) {
            goto free;
        }
        goto done;
    }
    sws_ctx = sws_getContext(frame->width, frame->height, frame->format,
      output_frame->width, output_frame->height, output_fmt,
      SWS_LANCZOS | SWS_ACCURATE_RND, NULL, NULL, NULL);
    if (!sws_ctx) {
        goto free;
    }
    if (sws_scale(sws_ctx, (const uint8_t *const *) frame->data, frame->linesize, 0, frame->height, output_frame->data,
                  output_frame->linesize) != output_frame->height) {
        goto free;
    } else {
        goto done;
    }
    free:
    av_frame_free(&output_frame);
    done:
    if (sws_ctx) {
        sws_freeContext(sws_ctx);
    }
    return output_frame;
}

AVPacket create_packet() {
    AVPacket *pkt = av_packet_alloc();
    pkt->data = NULL;
    pkt->size = 0;
    return *pkt;
}

int
obtain_next_frame(AVFormatContext *fmt_ctx, AVCodecContext *dec_ctx, int stream_index, AVPacket *pkt, AVFrame **frame) {
    int err = 0, retry = 0;
    if (!(*frame) && !(*frame = av_frame_alloc())) {
        err = AVERROR(ENOMEM);
        return err;
    }
    if ((err = avcodec_receive_frame(dec_ctx, *frame)) != AVERROR(EAGAIN)) {
        return err;
    }
    while (1) {
        if ((err = av_read_frame(fmt_ctx, pkt)) < 0) {
            break;
        }
        if (pkt->stream_index != stream_index) {
            av_packet_unref(pkt);
            continue;
        }
        if ((err = avcodec_send_packet(dec_ctx, pkt)) < 0) {
            if (retry++ >= 10) {
                break;
            }
            continue;
        }
        if (!(*frame) && !(*frame = av_frame_alloc())) {
            err = AVERROR(ENOMEM);
            break;
        }
        err = avcodec_receive_frame(dec_ctx, *frame);
        if (err >= 0 || err != AVERROR(EAGAIN)) {
            break;
        }
        av_packet_unref(pkt);
    }
    if (pkt->buf) {
        av_packet_unref(pkt);
    }
    return err;
}

int64_t find_duration(AVFormatContext *fmt_ctx) {
    AVPacket pkt = create_packet();
    int err = 0;
    int64_t duration = 0;
    while (err >= 0) {
        err = av_read_frame(fmt_ctx, &pkt);
        if (pkt.pts != AV_NOPTS_VALUE) {
            AVRational time_base = fmt_ctx->streams[pkt.stream_index]->time_base;
            duration = FFMAX(duration, pkt.pts * 1000000000 * time_base.num / time_base.den);
        }
        av_packet_unref(&pkt);
    }
    if (err == AVERROR_EOF) {
        return duration;
    }
    return err;
}

ThumbContext *create_thumb_context(AVStream *stream, AVFrame *frame) {
    ThumbContext *thumb_ctx = av_mallocz(sizeof *thumb_ctx);
    if (!thumb_ctx) {
        return thumb_ctx;
    }
//    thumb_ctx->n = 0;
    thumb_ctx->desc = av_pix_fmt_desc_get(frame->format);
    int nb_frames = 100;
    if (stream->disposition & AV_DISPOSITION_ATTACHED_PIC) {
        nb_frames = 1;
    } else if (stream->nb_frames && stream->nb_frames < 400) {
        nb_frames = (int) (stream->nb_frames >> 2) + 1;
    }
    int frames_in_128mb = (1 << 30) / (av_get_bits_per_pixel(thumb_ctx->desc) * frame->height * frame->width);
    thumb_ctx->max_frames = FFMIN(nb_frames, frames_in_128mb);
    int i;
    for (i = 0; i < thumb_ctx->desc->nb_components; i++) {
        thumb_ctx->hist_size += 1 << thumb_ctx->desc->comp[i].depth;
    }
    thumb_ctx->median = av_calloc(thumb_ctx->hist_size, sizeof(double));
    if (!thumb_ctx->median) {
        av_free(thumb_ctx);
        return NULL;
    }
    thumb_ctx->frames = av_malloc_array((size_t) thumb_ctx->max_frames, sizeof *thumb_ctx->frames);
    if (!thumb_ctx->frames) {
        av_free(thumb_ctx->median);
        av_free(thumb_ctx);
        return NULL;
    }
    for (i = 0; i < thumb_ctx->max_frames; i++) {
        thumb_ctx->frames[i].frame = NULL;
        thumb_ctx->frames[i].hist = av_calloc(thumb_ctx->hist_size, sizeof(int));
        if (!thumb_ctx->frames[i].hist) {
            for (i--; i >= 0; i--) {
                av_free(thumb_ctx->frames[i].hist);
            }
            av_free(thumb_ctx->median);
            av_free(thumb_ctx);
            return NULL;
        }
    }
    return thumb_ctx;
}

void free_thumb_context(ThumbContext *thumb_ctx) {
    if (!thumb_ctx) {
        return;
    }
    int i;
    for (i = 0; i < thumb_ctx->n; i++) {
        av_frame_free(&thumb_ctx->frames[i].frame);
        av_free(thumb_ctx->frames[i].hist);
    }
    for (i = thumb_ctx->n; i < thumb_ctx->max_frames; i++) {
        av_free(thumb_ctx->frames[i].hist);
    }
    av_free(thumb_ctx->median);
    av_free(thumb_ctx->frames);
    av_free(thumb_ctx);
}

static double root_mean_square_error(const int *hist, const double *median, size_t hist_size) {
    int i;
    double err, sum_sq_err = 0;
    for (i = 0; i < hist_size; i++) {
        err = median[i] - (double) hist[i];
        sum_sq_err += err * err;
    }
    return sum_sq_err;
}

void populate_frame(ThumbContext *thumb_ctx, int n, AVFrame *frame) {
    thumb_ctx->frames[n].frame = frame;
}

void populate_histogram(ThumbContext *thumb_ctx, int n, AVFrame *frame) {
    const AVPixFmtDescriptor *desc = thumb_ctx->desc;
    thumb_ctx->frames[n].frame = frame;
    int *hist = thumb_ctx->frames[n].hist;
    AVComponentDescriptor comp;
    int w, h, plane, depth, mask, shift, step, height, width;
    uint64_t flags;
    uint8_t **data = frame->data;
    int *linesize = frame->linesize;
    for (int c = 0; c < desc->nb_components; c++) {
        comp = desc->comp[c];
        plane = comp.plane;
        depth = comp.depth;
        mask = (1 << depth) - 1;
        shift = comp.shift;
        step = comp.step;
        flags = desc->flags;
        width = !(desc->log2_chroma_w) || (c != 1 && c != 2) ? frame->width : AV_CEIL_RSHIFT(frame->width,
                                                                                             desc->log2_chroma_w);
        height = !(desc->log2_chroma_h) || (c != 1 && c != 2) ? frame->height : AV_CEIL_RSHIFT(frame->height,
                                                                                               desc->log2_chroma_h);
        for (h = 0; h < height; h++) {
            w = width;
            if (flags & AV_PIX_FMT_FLAG_BITSTREAM) {
                const uint8_t *p = data[plane] + h * linesize[plane] + (comp.offset >> 3);
                shift = 8 - depth - (comp.offset & 7);

                while (w--) {
                    int val = (*p >> shift) & mask;
                    shift -= step;
                    p -= shift >> 3;
                    shift &= 7;
                    (*(hist + val))++;
                }
            } else {
                const uint8_t *p = data[plane] + h * linesize[plane] + comp.offset;
                int is_8bit = shift + depth <= 8;

                if (is_8bit)
                    p += (flags & AV_PIX_FMT_FLAG_BE) != 0;

                while (w--) {
                    int val = is_8bit ? *p :
                              flags & AV_PIX_FMT_FLAG_BE ? AV_RB16(p) : AV_RL16(p);
                    val = (val >> shift) & mask;
                    p += step;
                    (*(hist + val))++;
                }
            }
        }
        hist += 1 << depth;
    }
}

int find_best_frame_index(ThumbContext *thumb_ctx) {
    int i, j, n = 0, m = thumb_ctx->n, *hist = NULL;
    double *median = thumb_ctx->median;
    for (j = 0; j < m; j++) {
        hist = thumb_ctx->frames[j].hist;
        for (i = 0; i < thumb_ctx->hist_size; i++) {
            median[i] += (double) hist[i] / m;
        }
    }
    struct thumb_frame *t_frame = NULL;
    double min_sum_sq_err = DBL_MAX, sum_sq_err = 0;
    for (i = 0; i < thumb_ctx->n; i++) {
        t_frame = thumb_ctx->frames + i;
        sum_sq_err = root_mean_square_error(t_frame->hist, thumb_ctx->median, thumb_ctx->hist_size);
        if (sum_sq_err < min_sum_sq_err) {
            min_sum_sq_err = sum_sq_err;
            n = i;
        }
    }
    return n;
}

AVFrame *select_frame(ThumbContext *thumb_ctx, int n) {
    return thumb_ctx->frames[n].frame;
}