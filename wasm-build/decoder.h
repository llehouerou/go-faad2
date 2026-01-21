#ifndef FAAD2_DECODER_H
#define FAAD2_DECODER_H

#ifdef __cplusplus
extern "C" {
#endif

// Version info
const char* faad2_version(void);

// Decoder lifecycle
void* faad2_decoder_create(void);
void faad2_decoder_destroy(void* decoder);

// Initialize decoder with AAC codec config (from MP4 esds box)
// Returns: 0 on success, negative on error
int faad2_decoder_init(void* decoder, unsigned char* config, unsigned int config_len,
                       unsigned long* sample_rate, unsigned char* channels);

// Decode a single AAC frame
// Returns: number of samples decoded, or negative on error
int faad2_decoder_decode(void* decoder,
                         unsigned char* aac_data, unsigned int aac_size,
                         short* pcm_out, unsigned int pcm_out_size);

// Get last error message
const char* faad2_get_error(void* decoder);

#ifdef __cplusplus
}
#endif

#endif // FAAD2_DECODER_H
