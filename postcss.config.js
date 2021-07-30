const tailwind = require('tailwindcss');
const purgecss = require('@fullhuman/postcss-purgecss');
const cssnano = require('cssnano');

const isProduction = process.env.NODE_ENV === 'production';

module.exports = {
  plugins: [
    tailwind('./tailwind.config.js'),
    ...(isProduction ? [
      purgecss({
        content: [
          './server/templates/**/*.html.tmpl',
        ],
        defaultExtractor: content => {
          return content.match(/[\w-./]*\w/g) || [];
        },
        // status classes are dynamically generated
        safelist: [
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
