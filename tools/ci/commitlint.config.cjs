module.exports = {
    extends: ['@commitlint/config-conventional'],
    rules: {
        // Disables line-length restrictions for commit bodies to permit detailed technical context.
        // Header length SHALL remain constrained by the default @commitlint/config-conventional 100-character limit.
        'body-max-line-length': [0, 'always', Infinity],
        // Appends `adhoc` to the default type-enum list for alignment with the `type::adhoc` group label.
        // `internal/semver.DetermineBump` SHALL treat `adhoc` commits as non-releasable change types.
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
                'adhoc',
            ],
        ],
    },
};
