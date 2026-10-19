export interface Article {
    ID: string;
    Title: string;
    Preview: string;
    Content: string;
    CreatedAt?: string;
    Username?: string;  // 作者用户名
    UserID?: number;    // 作者ID
    LikesCount?: number; // 点赞数
}

export interface Like{
    likes: number;
    is_liked: boolean;
}

export interface Comment {
    ID: string;
    article_id: number;
    user_id: number;
    username: string;
    content: string;
    parent_id?: number;
    replies?: Comment[];
    CreatedAt: string;
}

export interface CommentCount {
    count: number;
}

export interface CommentsResponse {
    data: Comment[];
    total: number;
    page: number;
    pageSize: number;
}
