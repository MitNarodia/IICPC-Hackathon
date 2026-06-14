/// metrics/hdr_histogram.cpp
/// =========================
/// Implementation of the HDR histogram.
/// The key insight: bucket index is computed from the magnitude (leading zeros)
/// of the value, giving logarithmic range coverage with linear sub-bucket precision.

#include "metrics/hdr_histogram.hpp"
#include <stdexcept>
#include <bit>
#include <iostream>

namespace bot_fleet::metrics {

HdrHistogram::HdrHistogram(int64_t min_value_us, int64_t max_value_us, int significant_digits)
    : min_value_(min_value_us)
    , max_value_(max_value_us)
    , significant_digits_(significant_digits)
{
    // Sub-bucket count determines precision within each power-of-2 range.
    // 2^10 = 1024 sub-buckets gives ~3 significant digits of precision.
    int sub_bucket_half_count_magnitude = significant_digits + 1;
    sub_bucket_count_ = 1 << sub_bucket_half_count_magnitude; // e.g., 2048

    // Number of power-of-2 ranges needed to cover [min, max]
    int64_t range = max_value_us / min_value_us;
    bucket_count_ = 1;
    while ((1LL << bucket_count_) < range) {
        ++bucket_count_;
    }
    bucket_count_ += 1;

    total_buckets_ = (bucket_count_ + 1) * (sub_bucket_count_ / 2);
    counts_.resize(total_buckets_, 0);
}

void HdrHistogram::record(int64_t value_us) {
    if (value_us < min_value_) value_us = min_value_;
    if (value_us > max_value_) value_us = max_value_;

    int idx = bucket_index(value_us);
    if (idx >= 0 && idx < total_buckets_) {
        ++counts_[idx];
        ++total_count_;
        total_sum_ += value_us;
    }
}

int64_t HdrHistogram::percentile(double p) const {
    if (total_count_ == 0) return 0;

    double target = (p / 100.0) * total_count_;
    int64_t cumulative = 0;

    for (int i = 0; i < total_buckets_; ++i) {
        cumulative += counts_[i];
        if (static_cast<double>(cumulative) >= target) {
            return value_at_index(i);
        }
    }
    return max_value_;
}

double HdrHistogram::mean() const {
    if (total_count_ == 0) return 0.0;
    return static_cast<double>(total_sum_) / static_cast<double>(total_count_);
}

void HdrHistogram::reset() {
    std::fill(counts_.begin(), counts_.end(), 0);
    total_count_ = 0;
    total_sum_ = 0;
}

void HdrHistogram::merge(const HdrHistogram& other) {
    int merge_count = std::min(total_buckets_, other.total_buckets_);
    for (int i = 0; i < merge_count; ++i) {
        counts_[i] += other.counts_[i];
    }
    total_count_ += other.total_count_;
    total_sum_ += other.total_sum_;
}

int HdrHistogram::bucket_index(int64_t value) const {
    // Determine which power-of-2 bucket this value falls in
    int leading = 63 - std::countl_zero(static_cast<uint64_t>(value));
    // int sub_bucket_idx = static_cast<int>(value >> std::max(0, leading - (significant_digits_ + 1)));
    int shift = std::max(0, leading - (significant_digits_ + 1));

    int sub_bucket_idx =
        static_cast<int>(value >> shift);

    // Normalize into [sub_bucket_count_/2, sub_bucket_count_)
    while (sub_bucket_idx >= sub_bucket_count_) {
        sub_bucket_idx >>= 1;
        ++leading;
    }
    int bucket_base = leading * (sub_bucket_count_ / 2);
    int idx = bucket_base + (sub_bucket_idx - (sub_bucket_count_ / 2));
    return std::clamp(idx, 0, total_buckets_ - 1);
}

int64_t HdrHistogram::value_at_index(int index) const {
    // Inverse of bucket_index: find the representative value for a bucket
    int bucket = index / (sub_bucket_count_ / 2);
    int sub = index % (sub_bucket_count_ / 2) + (sub_bucket_count_ / 2);
    int shift = std::max(0, bucket - (significant_digits_ + 1));
    std::cerr
        << "index=" << index
        << " bucket=" << bucket
        << " sub=" << sub
        << " shift=" << shift
        << " value=" << (static_cast<int64_t>(sub) << shift)
        << std::endl;
    // return static_cast<int64_t>(sub) << shift;
    int64_t lower = static_cast<int64_t>(sub) << shift;
    int64_t width = static_cast<int64_t>(1) << shift;
    return lower + width / 2;

}

} // namespace bot_fleet::metrics
