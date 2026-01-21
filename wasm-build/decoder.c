#include "decoder.h"
#include <neaacdec.h>
#include <string.h>
#include <stdlib.h>

// Decoder context wrapper
typedef struct {
    NeAACDecHandle handle;
    char error_msg[256];
} DecoderContext;

const char* faad2_version(void) {
    return FAAD2_VERSION;
}

void* faad2_decoder_create(void) {
    DecoderContext* ctx = (DecoderContext*)malloc(sizeof(DecoderContext));
    if (!ctx) {
        return NULL;
    }

    ctx->handle = NeAACDecOpen();
    if (!ctx->handle) {
        free(ctx);
        return NULL;
    }

    ctx->error_msg[0] = '\0';

    // Configure decoder for 16-bit output
    NeAACDecConfigurationPtr config = NeAACDecGetCurrentConfiguration(ctx->handle);
    config->outputFormat = FAAD_FMT_16BIT;
    config->downMatrix = 0;
    NeAACDecSetConfiguration(ctx->handle, config);

    return ctx;
}

void faad2_decoder_destroy(void* decoder) {
    if (!decoder) {
        return;
    }

    DecoderContext* ctx = (DecoderContext*)decoder;
    if (ctx->handle) {
        NeAACDecClose(ctx->handle);
    }
    free(ctx);
}

int faad2_decoder_init(void* decoder, unsigned char* config, unsigned int config_len,
                       unsigned long* sample_rate, unsigned char* channels) {
    if (!decoder || !config || config_len == 0) {
        return -1;
    }

    DecoderContext* ctx = (DecoderContext*)decoder;

    int result = NeAACDecInit2(ctx->handle, config, config_len, sample_rate, channels);
    if (result < 0) {
        snprintf(ctx->error_msg, sizeof(ctx->error_msg), "Init failed with code %d", result);
        return -1;
    }

    return 0;
}

int faad2_decoder_decode(void* decoder,
                         unsigned char* aac_data, unsigned int aac_size,
                         short* pcm_out, unsigned int pcm_out_size) {
    if (!decoder || !aac_data || aac_size == 0 || !pcm_out) {
        return -1;
    }

    DecoderContext* ctx = (DecoderContext*)decoder;
    NeAACDecFrameInfo frame_info;

    void* sample_buffer = NeAACDecDecode(ctx->handle, &frame_info, aac_data, aac_size);

    if (frame_info.error != 0) {
        snprintf(ctx->error_msg, sizeof(ctx->error_msg), "%s",
                 NeAACDecGetErrorMessage(frame_info.error));
        return -1;
    }

    if (!sample_buffer || frame_info.samples == 0) {
        return 0;
    }

    // Calculate bytes to copy
    unsigned int samples_to_copy = frame_info.samples;
    unsigned int bytes_to_copy = samples_to_copy * sizeof(short);

    if (bytes_to_copy > pcm_out_size) {
        samples_to_copy = pcm_out_size / sizeof(short);
        bytes_to_copy = samples_to_copy * sizeof(short);
    }

    memcpy(pcm_out, sample_buffer, bytes_to_copy);

    return (int)samples_to_copy;
}

const char* faad2_get_error(void* decoder) {
    if (!decoder) {
        return "Invalid decoder";
    }

    DecoderContext* ctx = (DecoderContext*)decoder;
    return ctx->error_msg;
}
