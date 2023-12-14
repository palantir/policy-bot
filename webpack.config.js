const path = require('path');
const MiniCssExtractPlugin = require('mini-css-extract-plugin');
const { WebpackManifestPlugin } = require('webpack-manifest-plugin');

const isProduction = process.env.NODE_ENV === "production";
const hashStr = isProduction ? '-[contenthash:10]' : '';

module.exports = {
  mode: isProduction ? 'production' : 'development',
  entry: {
    main: './server/assets/index.js',
    htmx: 'htmx.org',
  },
  output: {
    filename: `js/[name]${hashStr}.js`,
    path: path.resolve(__dirname, 'build', 'static'),
    clean: true,
  },
  plugins: [
    new MiniCssExtractPlugin({
      filename: `css/[name]${hashStr}.css`,
    }),
    ...(isProduction ? [
      new WebpackManifestPlugin({
        publicPath: '',
        generate: (seed, files, entrypoints) => {
          return files.reduce(
            (manifest, file) => {
              const key = path.join(path.dirname(file.path), path.basename(file.name));
              return Object.assign(manifest, { [key]: file.path })
            },
            seed
          );
        },
      }),
    ] : []),
  ],
  module: {
    rules: [
      {
        test: /\.css$/i,
        use: [MiniCssExtractPlugin.loader, 'css-loader', 'postcss-loader'],
      },
      {
        test: /\.(ico|svg)$/i,
        type: 'asset/resource',
        generator: {
          filename: `img/[name]${isProduction ? '-[hash:10]' : ''}[ext]`,
        },
      },
    ]
  },
}
