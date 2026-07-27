module.exports = {
    extends: ['@commitlint/config-conventional'],
    rules: {
        // Header length remains constrained by the default @commitlint/config-conventional threshold (100 characters).
        // Disables line-length restrictions for the commit body to support detailed technical descriptions
        'body-max-line-length': [0, 'always', Infinity],
    },
};
