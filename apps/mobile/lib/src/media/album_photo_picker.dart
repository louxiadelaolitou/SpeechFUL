import 'dart:async';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:photo_manager/photo_manager.dart';

import '../design_system/motion.dart';
import '../design_system/motion_routes.dart';
import '../design_system/telegram_theme.dart';
import '../localization/app_localizations.dart';
import '../posts/post_photo_picker.dart';

/// Opens the shared album sheet from the calling route. Existing platform
/// pickers are used when album access is unavailable.
class AlbumPhotoPicker implements PostPhotoPicker {
  static const _cancelled = (files: <PostPickedFile>[], fromSystem: false);

  AlbumPhotoPicker(
    this.context, {
    PostPhotoPicker? systemPicker,
    this.avatar = false,
  }) : _systemPicker = systemPicker ?? NativePostPhotoPicker();

  final BuildContext context;
  final bool avatar;
  final PostPhotoPicker _systemPicker;

  static PostPhotoPicker forView(
    BuildContext context,
    PostPhotoPicker configured,
  ) => configured is NativePostPhotoPicker
      ? AlbumPhotoPicker(context, systemPicker: configured)
      : configured;

  @override
  Future<void> discardUnboundLostSelection() =>
      _systemPicker.discardUnboundLostSelection();

  @override
  Future<List<PostPickedFile>> pick(int limit) async =>
      (await pickWithSource(limit)).files;

  Future<({List<PostPickedFile> files, bool fromSystem})> pickWithSource(
    int limit, {
    bool preferSystem = false,
  }) async {
    if (limit < 1 || limit > 9) throw RangeError.range(limit, 1, 9);
    if (!context.mounted) return _cancelled;
    var fromSystem = preferSystem;
    if (!fromSystem) {
      try {
        final permission = await PhotoManager.requestPermissionExtend(
          requestOption: const PermissionRequestOption(
            androidPermission: AndroidPermission(
              type: RequestType.image,
              mediaLocation: false,
            ),
          ),
        );
        if (!context.mounted) return _cancelled;
        fromSystem = !permission.hasAccess;
      } on Object {
        fromSystem = true;
      }
      if (!fromSystem) {
        final result = await showModalBottomSheet<Object>(
          context: context,
          isScrollControlled: true,
          backgroundColor: HhTelegramPalette.of(context).surface,
          builder: (_) => _AlbumPickerSheet(limit: limit, avatar: avatar),
        );
        if (!context.mounted || result == null) return _cancelled;
        if (result is List<AssetEntity>) {
          return (files: result.map(_asPickedFile).toList(), fromSystem: false);
        }
        fromSystem = result == _useSystem;
      }
    }
    if (fromSystem && context.mounted) {
      final files = await _systemPicker.pick(limit);
      if (!context.mounted) {
        for (final file in files) {
          await file.release?.call();
        }
        return _cancelled;
      }
      return (files: files, fromSystem: true);
    }
    return _cancelled;
  }

  static PostPickedFile _asPickedFile(AssetEntity asset) {
    Future<File>? original;
    Future<File> resolve() => original ??= () async {
      final file = await asset.originFile;
      if (file == null) throw const PostPhotoSelectionUnavailable();
      return file;
    }();
    return PostPickedFile(
      length: () async => (await resolve()).length(),
      openRead: () async* {
        yield* (await resolve()).openRead();
      },
    );
  }
}

const _useSystem = 'use-system-picker';

class _AlbumPickerSheet extends StatefulWidget {
  const _AlbumPickerSheet({required this.limit, required this.avatar});

  final int limit;
  final bool avatar;

  @override
  State<_AlbumPickerSheet> createState() => _AlbumPickerSheetState();
}

class _AlbumPickerSheetState extends State<_AlbumPickerSheet> {
  final _controller = DraggableScrollableController();

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => DraggableScrollableSheet(
    expand: false,
    initialChildSize: .62,
    minChildSize: .3,
    maxChildSize: 1,
    snap: true,
    snapSizes: const [.62],
    controller: _controller,
    builder: (context, scrollController) => _AlbumSheet(
      scrollController: scrollController,
      sheetController: _controller,
      limit: widget.limit,
      avatar: widget.avatar,
    ),
  );
}

class _AlbumSheet extends StatefulWidget {
  const _AlbumSheet({
    required this.scrollController,
    required this.sheetController,
    required this.limit,
    required this.avatar,
  });
  final ScrollController scrollController;
  final DraggableScrollableController sheetController;
  final int limit;
  final bool avatar;
  @override
  State<_AlbumSheet> createState() => _AlbumSheetState();
}

class _AlbumSheetState extends State<_AlbumSheet> {
  List<AssetEntity>? _assets;
  List<AssetPathEntity>? _albums;
  AssetPathEntity? _album;
  int? _albumCount;
  int _albumCountRequest = 0;
  final _selection = <String, AssetEntity>{};

  void _select(AssetEntity asset) {
    if (widget.limit == 1) {
      Navigator.pop(context, <AssetEntity>[asset]);
    } else if (_selection.containsKey(asset.id)) {
      setState(() => _selection.remove(asset.id));
    } else if (_selection.length < widget.limit) {
      setState(() => _selection[asset.id] = asset);
    }
  }

  Object? _error;
  int _page = 0;
  bool _loadingMore = false;
  bool _hasMore = true;
  bool _menuOpen = false;
  static const _pageSize = 90;
  // 相册实体与分页查询共用创建时间降序，保证各页均按最新优先返回。
  static FilterOptionGroup _newestFirstFilter() => FilterOptionGroup(
    orders: const [OrderOption(type: OrderOptionType.createDate, asc: false)],
  );
  int _epoch = 0;
  @override
  void initState() {
    super.initState();
    // 先取相册列表（确定默认 isAll），再加载网格——保证初始与切换后
    // 走同一条实体分页查询，排序一致；列表失败时 _fetchPage 的空相册
    // 分支兜底。
    unawaited(_loadAlbums().then((_) => _load()));
  }

  // 设备相册分类（相机/截屏/收藏等），默认“全部照片”；切换后网格重载。
  Future<void> _loadAlbums() async {
    try {
      final albums = await PhotoManager.getAssetPathList(
        type: RequestType.image,
        hasAll: true,
        filterOption: _newestFirstFilter(),
      );
      if (!mounted) return;
      // “全部照片”固定置顶；其余保持系统返回顺序（Android 端本就在
      // 头部合成 isAll 相册，但名称硬编码为“Recent”，展示名在 UI 层
      // 覆盖为本地化的「全部照片」）。
      final sorted = [
        ...albums.where((album) => album.isAll),
        ...albums.where((album) => !album.isAll),
      ];
      AssetPathEntity? all;
      for (final album in sorted) {
        if (album.isAll) {
          all = album;
          break;
        }
      }
      all ??= sorted.isEmpty ? null : sorted.first;
      setState(() {
        _albums = sorted;
        if (_album == null && all != null) _album = all;
      });
      unawaited(_refreshAlbumCount());
    } catch (_) {
      // 相册列表加载失败不阻塞网格（继续按“全部照片”展示）。
    }
  }

  // 头部按钮展示的照片数；缓存避免随弹窗拖动的逐帧重建反复查询。
  Future<void> _refreshAlbumCount() async {
    final request = ++_albumCountRequest;
    final album = _album;
    if (album == null) return;
    try {
      final count = await album.assetCountAsync;
      if (mounted && request == _albumCountRequest) {
        setState(() => _albumCount = count);
      }
    } catch (_) {
      // 数量查询失败仅省略显示。
    }
  }

  void _selectAlbum(AssetPathEntity album) {
    if (album.id == _album?.id) return;
    setState(() {
      _album = album;
      _albumCount = null;
    });
    unawaited(_refreshAlbumCount());
    _load();
  }

  Future<List<AssetEntity>> _fetchPage(int page) async {
    final album = _album;
    return album == null
        ? PhotoManager.getAssetListPaged(
            page: page,
            pageCount: _pageSize,
            type: RequestType.image,
            filterOption: _newestFirstFilter(),
          )
        : album.getAssetListPaged(page: page, size: _pageSize);
  }

  Future<void> _load() async {
    if (!mounted) return;
    final epoch = ++_epoch;
    setState(() {
      _assets = null;
      _error = null;
      _page = 0;
      _hasMore = true;
      _loadingMore = false;
    });
    try {
      final items = await _fetchPage(0);
      if (!mounted || epoch != _epoch) return;
      setState(() {
        _assets = items;
        _hasMore = items.length >= _pageSize;
      });
    } catch (error) {
      if (!mounted || epoch != _epoch) return;
      setState(() => _error = error);
    }
  }

  // 分页懒加载：滚动接近底部时再取下一页。epoch 防切换相册后旧的
  // 在途请求把别的相册内容追加进来（顺序打乱的根因）；按 id 去重防
  // 分页边界抖动造成重复。
  Future<void> _loadMore() async {
    if (_loadingMore || !_hasMore || _assets == null || _error != null) {
      return;
    }
    final epoch = _epoch;
    setState(() => _loadingMore = true);
    try {
      final items = await _fetchPage(_page + 1);
      if (!mounted || epoch != _epoch) return;
      setState(() {
        _loadingMore = false;
        if (items.isEmpty) {
          _hasMore = false;
        } else {
          final known = _assets!.map((e) => e.id).toSet();
          final fresh = items.where((e) => !known.contains(e.id)).toList();
          _page++;
          _assets = [..._assets!, ...fresh];
          _hasMore = items.length >= _pageSize;
        }
      });
    } catch (_) {
      if (mounted && epoch == _epoch) {
        setState(() {
          _loadingMore = false;
          _hasMore = false;
        });
      }
    }
  }

  /// 下拉列表约束：宽度对齐按钮（整宽减去左右 16 内边距），高度随展开
  /// 进度自适应（半屏取较小上限、全屏取较大上限）。不跨树读取 RenderBox，
  /// 避免在 build 阶段访问已失效 element 触发断言。
  BoxConstraints _albumMenuConstraints() {
    final screen = MediaQuery.sizeOf(context);
    final width = screen.width - 32;
    // 按半屏高度保守估算按钮下方空间，避免访问 DraggableScrollableController。
    final maxHeight = (screen.height * .62 - 160).clamp(112.0, 376.0);
    return BoxConstraints(
      minWidth: width < 288.0 ? 288.0 : width,
      maxHeight: maxHeight,
    );
  }

  /// 扁平风相册选择按钮：整宽、底色块 + 细边框、图标 + 相册名 + 数量 +
  /// 下拉箭头；下拉面板同扁平（surface 底、1px 边框、零阴影、选中项带勾
  /// 与字重，不只靠颜色区分）。clipBehavior 使选中/悬停的高亮被裁剪到
  /// 面板圆角内，不再向外突出方角。isAll 相册展示名覆盖为「全部照片」
  /// （插件在 Android 上把该相册硬编码命名为“Recent”）。
  Widget _albumSelector(BuildContext context) {
    final p = HhTelegramPalette.of(context);
    final album = _album;
    String displayName(AssetPathEntity? item) =>
        item == null || item.isAll ? hhText(context, '全部照片') : item.name;
    return Material(
      color: p.background,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(12),
        side: BorderSide(color: p.line),
      ),
      clipBehavior: Clip.antiAlias,
      child: PopupMenuButton<AssetPathEntity>(
        tooltip: hhText(context, '选择相册'),
        // 此参数约束按钮内部 InkWell；clipBehavior 仅作用于下拉菜单。
        borderRadius: BorderRadius.circular(12),
        initialValue: _album,
        onOpened: () => setState(() => _menuOpen = true),
        onCanceled: () => setState(() => _menuOpen = false),
        onSelected: (album) {
          setState(() => _menuOpen = false);
          _selectAlbum(album);
        },
        position: PopupMenuPosition.under,
        popUpAnimationStyle: HhMotionRoutes.popup(context),
        color: p.surface,
        elevation: 0,
        surfaceTintColor: Colors.transparent,
        clipBehavior: Clip.antiAlias,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(14),
          side: BorderSide(color: p.line),
        ),
        constraints: _albumMenuConstraints(),
        itemBuilder: (menuContext) => [
          for (final item in _albums ?? const <AssetPathEntity>[])
            PopupMenuItem(
              value: item,
              height: 48,
              padding: const EdgeInsets.symmetric(horizontal: 16),
              // 占满整行：菜单项内容被外层 Align(centerStart) 定位，
              // 固定宽度会让数量停在宽度内而非菜单右缘。
              child: Row(
                children: [
                  Icon(
                    item.id == album?.id
                        ? Icons.check_rounded
                        : Icons.photo_library_outlined,
                    size: 18,
                    color: item.id == album?.id ? p.accent : p.secondary,
                  ),
                  const SizedBox(width: 10),
                  Expanded(
                    child: Text(
                      displayName(item),
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: TextStyle(
                        fontSize: 14,
                        fontWeight: item.id == album?.id
                            ? FontWeight.w600
                            : FontWeight.w400,
                        color: p.ink,
                      ),
                    ),
                  ),
                  const SizedBox(width: 8),
                  FutureBuilder<int>(
                    future: item.assetCountAsync,
                    builder: (context, snapshot) => Text(
                      snapshot.data?.toString() ?? '',
                      style: TextStyle(fontSize: 12, color: p.secondary),
                    ),
                  ),
                ],
              ),
            ),
        ],
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 11),
          child: Row(
            children: [
              Icon(Icons.photo_library_outlined, size: 20, color: p.accent),
              const SizedBox(width: 10),
              Expanded(
                child: Text(
                  displayName(album),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: TextStyle(
                    fontSize: 15,
                    fontWeight: FontWeight.w600,
                    color: p.ink,
                  ),
                ),
              ),
              if (_albumCount != null) ...[
                Text(
                  '$_albumCount',
                  style: TextStyle(fontSize: 12, color: p.secondary),
                ),
                const SizedBox(width: 2),
              ],
              // 下拉列表打开时箭头反转向上，与身份切换等控件的动画节奏一致。
              AnimatedRotation(
                turns: _menuOpen ? .5 : 0,
                duration: HhMotion.duration(context, HhMotion.quick),
                child: Icon(
                  Icons.arrow_drop_down_rounded,
                  size: 22,
                  color: p.secondary,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final p = HhTelegramPalette.of(context);
    final assets = _assets;
    final grid = NotificationListener<ScrollNotification>(
      // 接近底部（约两屏）时预取下一页，实现全部照片的分页懒加载。
      onNotification: (notice) {
        if (notice.metrics.extentAfter < 600) {
          unawaited(_loadMore());
        }
        return false;
      },
      child: CustomScrollView(
        // 挂 DraggableScrollableSheet 的控制器：网格滚到顶后再上滑即展开
        // 弹窗，下滑即收起，全屏/半屏均跟手。
        controller: widget.scrollController,
        slivers: [
          // 仅头部随展开进度重建（全屏时避开状态栏），网格缩略图不受影响。
          SliverToBoxAdapter(
            // 头部内容固定为 child，展开进度只驱动外层 Padding，避免拖动
            // 逐帧重建相册按钮、箭头与文案，减少无谓重排。
            child: AnimatedBuilder(
              animation: widget.sheetController,
              builder: (context, _) {
                final expand = widget.sheetController.isAttached
                    ? ((widget.sheetController.size - .62) / .38).clamp(
                        0.0,
                        1.0,
                      )
                    : 0.0;
                return Padding(
                  padding: EdgeInsets.only(
                    top: MediaQuery.paddingOf(context).top * expand,
                  ),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      const SizedBox(height: 10),
                      Center(
                        child: Container(
                          width: 36,
                          height: 4,
                          decoration: BoxDecoration(
                            color: p.line,
                            borderRadius: BorderRadius.circular(2),
                          ),
                        ),
                      ),
                      Padding(
                        padding: const EdgeInsets.fromLTRB(20, 12, 8, 0),
                        child: Row(
                          children: [
                            Expanded(
                              child: Text(
                                hhText(
                                  context,
                                  widget.avatar ? '设置头像' : '选择照片',
                                ),
                                style: TextStyle(
                                  fontSize: 17,
                                  fontWeight: FontWeight.w600,
                                  color: p.ink,
                                ),
                              ),
                            ),
                            IconButton(
                              tooltip: hhText(context, '改用系统选择器'),
                              onPressed: () =>
                                  Navigator.pop(context, _useSystem),
                              icon: const Icon(Icons.photo_library_outlined),
                            ),
                            IconButton(
                              tooltip: hhText(context, '关闭'),
                              onPressed: () => Navigator.pop(context),
                              icon: const Icon(Icons.close_rounded),
                            ),
                          ],
                        ),
                      ),
                      if (widget.avatar)
                        Padding(
                          padding: const EdgeInsets.fromLTRB(20, 0, 20, 8),
                          child: Text(
                            hhText(context, '上滑展开全屏，选择照片后进入圆形裁切。'),
                            style: TextStyle(fontSize: 13, color: p.secondary),
                          ),
                        ),
                      Padding(
                        padding: const EdgeInsets.fromLTRB(16, 0, 16, 10),
                        child: _albumSelector(context),
                      ),
                    ],
                  ),
                );
              },
            ),
          ),
          if (_error != null)
            SliverFillRemaining(
              hasScrollBody: false,
              child: Padding(
                padding: const EdgeInsets.all(24),
                child: Column(
                  mainAxisAlignment: MainAxisAlignment.center,
                  children: [
                    Text(
                      hhText(context, '无法读取相册，请重试。'),
                      style: TextStyle(color: p.error),
                    ),
                    const SizedBox(height: 12),
                    Wrap(
                      alignment: WrapAlignment.center,
                      spacing: 12,
                      children: [
                        TextButton(
                          onPressed: _load,
                          child: Text(hhText(context, '重新加载')),
                        ),
                        TextButton(
                          onPressed: () => Navigator.pop(context, _useSystem),
                          child: Text(hhText(context, '改用系统选择器')),
                        ),
                      ],
                    ),
                  ],
                ),
              ),
            )
          else if (assets == null)
            const SliverFillRemaining(
              hasScrollBody: false,
              child: Center(
                child: Padding(
                  padding: EdgeInsets.all(24),
                  child: CircularProgressIndicator(),
                ),
              ),
            )
          else if (assets.isEmpty)
            SliverFillRemaining(
              hasScrollBody: false,
              child: Center(
                child: Text(
                  hhText(context, '相册里还没有照片'),
                  style: TextStyle(color: p.secondary),
                ),
              ),
            )
          else
            SliverPadding(
              padding: EdgeInsets.fromLTRB(
                2,
                4,
                2,
                12 + MediaQuery.paddingOf(context).bottom,
              ),
              sliver: SliverGrid(
                gridDelegate: const SliverGridDelegateWithFixedCrossAxisCount(
                  crossAxisCount: 4,
                  mainAxisSpacing: 4,
                  crossAxisSpacing: 4,
                ),
                delegate: SliverChildBuilderDelegate((context, index) {
                  final asset = assets[index];
                  final selected = _selection.containsKey(asset.id);
                  final enabled = selected || _selection.length < widget.limit;
                  return Semantics(
                    button: true,
                    selected: widget.limit > 1 ? selected : null,
                    enabled: enabled,
                    label: hhText(context, '照片 {v0}', args: {'v0': index + 1}),
                    child: GestureDetector(
                      key: ValueKey('album-photo-${asset.id}'),
                      onTap: enabled ? () => _select(asset) : null,
                      child: Stack(
                        fit: StackFit.expand,
                        children: [
                          _AlbumThumb(asset: asset),
                          if (widget.limit > 1)
                            Positioned(
                              right: 4,
                              top: 4,
                              child: Icon(
                                selected
                                    ? Icons.check_circle
                                    : Icons.radio_button_unchecked,
                                color: selected ? p.accent : Colors.white,
                              ),
                            ),
                        ],
                      ),
                    ),
                  );
                }, childCount: assets.length),
              ),
            ),
          if (_loadingMore)
            const SliverToBoxAdapter(
              child: Padding(
                padding: EdgeInsets.all(16),
                child: Center(
                  child: SizedBox(
                    width: 20,
                    height: 20,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  ),
                ),
              ),
            ),
        ],
      ),
    );
    return Column(
      children: [
        Expanded(child: grid),
        if (widget.limit > 1)
          SafeArea(
            top: false,
            child: Padding(
              padding: const EdgeInsets.fromLTRB(16, 6, 16, 8),
              child: Row(
                children: [
                  Expanded(
                    child: Text('${_selection.length} / ${widget.limit}'),
                  ),
                  FilledButton(
                    key: const ValueKey('album-confirm-selection'),
                    onPressed: _selection.isEmpty
                        ? null
                        : () => Navigator.pop(
                            context,
                            _selection.values.toList(),
                          ),
                    child: Text(hhText(context, '确定')),
                  ),
                ],
              ),
            ),
          ),
      ],
    );
  }
}

class _AlbumThumb extends StatefulWidget {
  const _AlbumThumb({required this.asset});
  final AssetEntity asset;

  @override
  State<_AlbumThumb> createState() => _AlbumThumbState();
}

class _AlbumThumbState extends State<_AlbumThumb> {
  late Future<Uint8List?> _thumbnail = _loadThumbnail();

  Future<Uint8List?> _loadThumbnail() =>
      widget.asset.thumbnailDataWithSize(const ThumbnailSize(200, 200));

  @override
  void didUpdateWidget(covariant _AlbumThumb oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.asset.id != widget.asset.id) _thumbnail = _loadThumbnail();
  }

  @override
  Widget build(BuildContext context) {
    return FutureBuilder<Uint8List?>(
      future: _thumbnail,
      builder: (context, snapshot) {
        final bytes = snapshot.data;
        if (bytes == null) {
          return ColoredBox(
            color: HhTelegramPalette.of(context).line,
            child: Center(
              child: snapshot.connectionState == ConnectionState.done
                  ? const Icon(Icons.broken_image_outlined, size: 20)
                  : const SizedBox(
                      width: 16,
                      height: 16,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    ),
            ),
          );
        }
        return Image.memory(bytes, fit: BoxFit.cover, gaplessPlayback: true);
      },
    );
  }
}
