# Bounded loops

The first compiler accepts only compile-time bounded `for` loops using `<`/`<=` and `++`/`+= CONST`. The static iteration count must not exceed 64. Generated C adds `#pragma unroll` so verifier behavior remains predictable.
