module.exports = {
    extends: ['@commitlint/config-conventional'],
    rules: {
        // Header length remains constrained by the default @commitlint/config-conventional threshold (100 characters).
        // Disables line-length restrictions for the commit body to support detailed technical descriptions
        'body-max-line-length': [0, 'always', Infinity],
        // Extends the @commitlint/config-conventional@21.2.0 default type-enum with `ad-hoc`, aligned with
        // the group label `type::ad-hoc`. `internal/semver.DetermineBump` already treats it as a no-release type.
        'type-enum': [
            2,
            'always',
            [
                'build',
                'chore',
                'ci',
                'docs',
                'feat',
                'fix',
                'perf',
                'refactor',
                'revert',
                'style',
                'test',
                'ad-hoc',
            ],
        ],
    },
};
