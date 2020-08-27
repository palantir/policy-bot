const tailwind = require('tailwindcss');
const purgecss = require('@fullhuman/postcss-purgecss');
const cssnano = require('cssnano');

const isProduction = process.env.NODE_ENV === 'production';

module.exports = {
  plugins: [
    tailwind('./tailwind.config.js'),
    ...(isProduction ? [
      // https://tailwindcss.com/docs/controlling-file-size#setting-up-purge-css-manually
      purgecss({
        content: [
          './server/templates/**/*.html.tmpl',
        ],
        defaultExtractor: content => {
          const broadMatches = content.match(/[^<>"'`\s]*[^<>"'`\s:]/g) || []
          const innerMatches = content.match(/[^<>"'`\s.()]*[^<>"'`\s.():]/g) || []
          return broadMatches.concat(innerMatches)
        },
        // status classes are dynamically generated
        whitelist: [
          "approved", "disapproved", "pending", "skipped", "error",
        ],
      }),
      cssnano({
        preset: ['default', {
          // this breaks some tailwind utils, like directional borders
          mergeLonghand: false,
        }],
      }),
    ] : []),
  ],
};
