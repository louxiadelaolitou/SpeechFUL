import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:hnuhole_mobile/hnuhole_mobile.dart';
import 'package:hnuhole_mobile/src/posts/feed_api.dart';

import 'feed_ui_test.dart' as feed;
import 'identity_test_support.dart';
import 'login_home_test_support.dart';

const _oldAvatar = 'avatar-v1:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa';
const _newAvatar = 'avatar-v1:bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb';

FeedPost _postWithAuthor(PostAuthor author) {
  final base = feed.post();
  return FeedPost(
    postId: base.postId,
    channelId: base.channelId,
    title: base.title,
    author: author,
    publishedAt: base.publishedAt,
    body: base.body,
    viewCount: base.viewCount,
    imageCount: base.imageCount,
    tags: base.tags,
  );
}

const _author = PostAuthor(
  state: PostAuthorState.active,
  avatar: _oldAvatar,
  nickname: '账号A的身份',
);
const _nextAuthor = PostAuthor(
  state: PostAuthorState.active,
  avatar: _newAvatar,
  nickname: '账号A的身份',
);

void main() {
  testWidgets(
    'identity refresh updates displayed home feed avatar without reordering',
    (tester) async {
      final f = HomeTestFixture();
      f.postApi.items = [_postWithAuthor(_author)];
      await tester.pumpWidget(await f.create());
      await tester.pumpAndSettle();
      expect(f.posts.homeFeed.timelineItems.single.author.avatar, _oldAvatar);

      f.postApi.items = [_postWithAuthor(_nextAuthor)];
      await f.posts.refreshKnownPosts();
      await tester.pump();

      expect(f.posts.homeFeed.timelineItems.map((post) => post.postId), [
        feed.postId,
      ]);
      expect(f.posts.homeFeed.timelineItems.single.author.avatar, _newAvatar);
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox.shrink());
      await tester.pump();
    },
  );

  testWidgets('identity refresh updates loaded personal post avatar', (
    tester,
  ) async {
    final f = HomeTestFixture();
    final oldPost = _postWithAuthor(_author);
    f.identitiesApi.items = [
      identityTestItem(name: _author.nickname!, avatar: _oldAvatar),
    ];
    f.postApi.items = [oldPost];
    f.postApi.identityItems[identityTestId] = [
      OwnPostCard(post: oldPost, serverSortAt: oldPost.publishedAt),
    ];
    await tester.pumpWidget(await f.create());
    await tester.pumpAndSettle();
    await f.posts.loadPersonal();
    expect(f.posts.personalItems.single.post!.author.avatar, _oldAvatar);

    final nextPost = _postWithAuthor(_nextAuthor);
    f.postApi.items = [nextPost];
    f.identitiesApi.items = [
      identityTestItem(name: _author.nickname!, avatar: _newAvatar),
    ];
    await f.posts.refreshPersonalProjections();
    await tester.pump();

    expect(f.posts.personalIdentity!.avatar, _newAvatar);
    expect(f.posts.personalItems.single.post!.author.avatar, _newAvatar);
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox.shrink());
    await tester.pump();
  });
}
